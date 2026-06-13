package main

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/linclin/rql"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

var (
	db *gorm.DB
	// QueryParam is the name of the query string key.
	queryParam = "query"
	// MustNewParser panics if the configuration is invalid.
	queryParser = rql.MustNewParser(rql.Config{
		Model:    User{},
		FieldSep: ".",
	})
)

// User is the model in gorm's terminology.
type User struct {
	ID          uint      `gorm:"primary_key" rql:"filter,sort"`
	Admin       bool      `rql:"filter"`
	Name        string    `rql:"filter"`
	AddressName string    `rql:"filter"`
	CreatedAt   time.Time `rql:"filter,sort"`
}

func main() {
	var err error
	db, err = gorm.Open(sqlite.Open("test.db"), &gorm.Config{})
	must(err, "initialize db")
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	must(db.AutoMigrate(&User{}), "run migration")
	must(db.Create(&User{Name: "test"}).Error, "create test user")
	http.HandleFunc("/users", GetUsers)
	log.Fatal(http.ListenAndServe(":8080", nil))
	// Now, go to your terminal and run the folllowing commad in order to test the application:
	// curl --request POST --data '{"filter": {"name": {"$like": "t%st"}}}' http://localhost:8080/users
}

// GetUsers accepts the db query in either the body or the query string.
func GetUsers(w http.ResponseWriter, r *http.Request) {
	var users []User
	p, err := getDBQuery(r)
	if err != nil {
		io.WriteString(w, err.Error())
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	err = db.Where(p.FilterExp, p.FilterArgs).
		Offset(p.Offset).
		Limit(p.Limit).
		Order(p.Sort).
		Find(&users).Error
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if err := json.NewEncoder(w).Encode(users); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
}

// getDBQuery extracts the query blob from either the body or the query string
// and execute the parser.
func getDBQuery(r *http.Request) (*rql.Params, error) {
	var (
		b   []byte
		err error
	)
	if v := r.URL.Query().Get(queryParam); v != "" {
		b, err = base64.StdEncoding.DecodeString(v)
	} else {
		b, err = io.ReadAll(io.LimitReader(r.Body, 1<<12))
	}
	if err != nil {
		return nil, err
	}
	return queryParser.Parse(b)
}

// must panics if the error is not nil.
func must(err error, msg string) {
	if err != nil {
		log.Fatalf("failed to %s: %v", msg, err)
	}
}
