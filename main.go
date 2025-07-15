package main

import (
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"sync/atomic"

	"github.com/dr-check/chirpy/internal/database"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

type apiConfig struct {
	fileserverHits atomic.Int32
	database       *database.Queries
	env            string
}

func (c *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.fileserverHits.Add(1)
		next.ServeHTTP(w, r)
	})
}

func main() {

	godotenv.Load(".env")

	dbURL := os.Getenv("DB_URL")

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		panic(fmt.Sprintf("Failed to connect to database: %v", err))
	}

	dbQueries := database.New(db)

	sMux := http.NewServeMux()

	apiCfig := &apiConfig{
		fileserverHits: atomic.Int32{},
		database:       dbQueries,
		env:            os.Getenv("PLATFORM"),
	}

	handler := http.StripPrefix("/app/", http.FileServer(http.Dir(".")))

	sMux.Handle("/app/", apiCfig.middlewareMetricsInc(handler))

	sMux.HandleFunc("POST /api/users", apiCfig.handlerCreateUser)

	sMux.HandleFunc("POST /api/chirps", apiCfig.handlerChirp)

	sMux.HandleFunc("GET /api/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("200 OK\n"))
	})

	sMux.HandleFunc("GET /admin/metrics", func(w http.ResponseWriter, r *http.Request) {
		count := apiCfig.fileserverHits.Load()
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(fmt.Sprintf(`
	<html>

	<body>
		<h1>Welcome, Chirpy Admin</h1>
		<p>Chirpy has been visited %d times!</p>
	</body>

	</html>
		`, count)))
	})

	sMux.HandleFunc("POST /admin/reset", func(w http.ResponseWriter, r *http.Request) {
		apiCfig.fileserverHits = atomic.Int32{}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("200 OK\n"))

		if apiCfig.env == "dev" {
			err := dbQueries.DeleteUsers(r.Context())
			if err != nil {
				http.Error(w, "Failed to delete users", http.StatusInternalServerError)
				return
			}
		} else {
			w.WriteHeader(http.StatusForbidden)
		}
	})

	newServer := &http.Server{
		Addr:    ":8080",
		Handler: sMux,
	}

	err2 := newServer.ListenAndServe()
	if err2 != nil {
		panic(err2)
	}
}
