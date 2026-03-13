package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	_ "github.com/lib/pq"
	_ "time/tzdata" // embed IANA timezone database so it works in scratch/alpine images
)

var db *sql.DB

// redirectURLs maps each source to its destination with the correct channel label
var redirectURLs = map[string]string{
	"linkedin":  "https://sb-auth.skillsbuild.org/signup?ngo-id=0429&utm_campaign=Lynx_cat3_org31-growth-interview-KE-cand14-linkedin",
	"whatsapp":  "https://sb-auth.skillsbuild.org/signup?ngo-id=0429&utm_campaign=Lynx_cat3_org31-growth-interview-KE-cand14-whatsapp",
	"community": "https://sb-auth.skillsbuild.org/signup?ngo-id=0429&utm_campaign=Lynx_cat3_org31-growth-interview-KE-cand14-community",
}

type Click struct {
	ID        int       `json:"id"`
	Source    string    `json:"source"`
	IPAddress string    `json:"ip_address"`
	UserAgent string    `json:"user_agent"`
	CreatedAt time.Time `json:"created_at"`
}

type GroupedClick struct {
	Source     string    `json:"source"`
	IPAddress  string    `json:"ip_address"`
	UserAgent  string    `json:"user_agent"`
	Count      int       `json:"count"`
	FirstClick time.Time `json:"first_click"`
	LastClick  time.Time `json:"last_click"`
}

type Stats struct {
	Source string `json:"source"`
	Count  int    `json:"count"`
}

func initDB() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://postgres:password@localhost:5432/linktracker?sslmode=disable"
	}

	var err error
	db, err = sql.Open("postgres", dsn)
	if err != nil {
		log.Fatal("Failed to open database:", err)
	}

	if err = db.Ping(); err != nil {
		log.Fatal("Failed to connect to database:", err)
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS clicks (
			id         SERIAL PRIMARY KEY,
			source     VARCHAR(50)  NOT NULL,
			ip_address VARCHAR(100) NOT NULL,
			user_agent TEXT,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
		)
	`)
	if err != nil {
		log.Fatal("Failed to create table:", err)
	}

	log.Println("Database connected and table ready")
}

// getClientIP extracts the real IP, honouring reverse-proxy headers.
func getClientIP(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		// X-Forwarded-For may be "client, proxy1, proxy2" — take the first
		if ip, _, err := net.SplitHostPort(fwd); err == nil {
			return ip
		}
		return fwd
	}
	if real := r.Header.Get("X-Real-IP"); real != "" {
		return real
	}
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

// trackAndRedirect records the click then sends the user to the target URL.
func trackAndRedirect(source string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip := getClientIP(r)
		userAgent := r.Header.Get("User-Agent")

		_, err := db.Exec(
			`INSERT INTO clicks (source, ip_address, user_agent) VALUES ($1, $2, $3)`,
			source, ip, userAgent,
		)
		if err != nil {
			log.Printf("Error recording click [%s] from %s: %v", source, ip, err)
		} else {
			log.Printf("Click recorded: source=%s ip=%s", source, ip)
		}

		http.Redirect(w, r, redirectURLs[source], http.StatusFound)
	}
}

// handleStats returns per-source click totals as JSON.
func handleStats(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query(`
		SELECT source, COUNT(*) AS count
		FROM clicks
		GROUP BY source
	`)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	statMap := map[string]int{"linkedin": 0, "whatsapp": 0, "community": 0}
	for rows.Next() {
		var source string
		var count int
		if err := rows.Scan(&source, &count); err == nil {
			statMap[source] = count
		}
	}

	stats := []Stats{
		{Source: "linkedin", Count: statMap["linkedin"]},
		{Source: "whatsapp", Count: statMap["whatsapp"]},
		{Source: "community", Count: statMap["community"]},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// handleClicks returns the 100 most recent raw click records as JSON.
func handleClicks(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query(`
		SELECT id, source, ip_address, user_agent, created_at
		FROM clicks
		ORDER BY created_at DESC
		LIMIT 100
	`)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	clicks := []Click{}
	for rows.Next() {
		var c Click
		if err := rows.Scan(&c.ID, &c.Source, &c.IPAddress, &c.UserAgent, &c.CreatedAt); err == nil {
			clicks = append(clicks, c)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(clicks)
}

// handleGroupedClicks returns clicks grouped by (ip_address, source) with count and time range.
func handleGroupedClicks(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query(`
		SELECT
			source,
			ip_address,
			MAX(user_agent)  AS user_agent,
			COUNT(*)         AS count,
			MIN(created_at)  AS first_click,
			MAX(created_at)  AS last_click
		FROM clicks
		GROUP BY source, ip_address
		ORDER BY last_click DESC
		LIMIT 200
	`)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	grouped := []GroupedClick{}
	for rows.Next() {
		var g GroupedClick
		if err := rows.Scan(&g.Source, &g.IPAddress, &g.UserAgent, &g.Count, &g.FirstClick, &g.LastClick); err == nil {
			grouped = append(grouped, g)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(grouped)
}

// handleClickDetail returns all individual clicks for a given ip + source.
func handleClickDetail(w http.ResponseWriter, r *http.Request) {
	ip := r.URL.Query().Get("ip")
	source := r.URL.Query().Get("source")
	if ip == "" || source == "" {
		http.Error(w, "ip and source query params required", http.StatusBadRequest)
		return
	}

	rows, err := db.Query(`
		SELECT id, source, ip_address, user_agent, created_at
		FROM clicks
		WHERE ip_address = $1 AND source = $2
		ORDER BY created_at DESC
	`, ip, source)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	clicks := []Click{}
	for rows.Next() {
		var c Click
		if err := rows.Scan(&c.ID, &c.Source, &c.IPAddress, &c.UserAgent, &c.CreatedAt); err == nil {
			clicks = append(clicks, c)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(clicks)
}

func main() {
	loc, err := time.LoadLocation("Africa/Nairobi")
	if err != nil {
		log.Fatal("Failed to load timezone Africa/Nairobi:", err)
	}
	time.Local = loc

	initDB()

	mux := http.NewServeMux()

	// Tracking / redirect endpoints
	mux.HandleFunc("/linkedin", trackAndRedirect("linkedin"))
	mux.HandleFunc("/whatsapp", trackAndRedirect("whatsapp"))
	mux.HandleFunc("/community", trackAndRedirect("community"))

	// JSON API for the dashboard
	mux.HandleFunc("/api/stats", handleStats)
	mux.HandleFunc("/api/clicks", handleClicks)
	mux.HandleFunc("/api/clicks/grouped", handleGroupedClicks)
	mux.HandleFunc("/api/clicks/detail", handleClickDetail)

	// Serve the dashboard frontend
	mux.Handle("/", http.FileServer(http.Dir("./static")))

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("LinkTracker running on http://localhost:%s", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}
