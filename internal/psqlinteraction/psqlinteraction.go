package psqlinteraction

import (
	"fmt"

	"github.com/jackc/pgx"
	"github.com/kishenkoilya/metricsalerts/internal/memstorage"
)

func PingPSQL(psqlLine string) error {
	connConfig, err := pgx.ParseConnectionString(psqlLine)
	if err != nil {
		return err
	}
	db, err := pgx.Connect(connConfig)
	if err != nil {
		return err
	}
	defer db.Close()
	return nil
}

type DBConnection struct {
	conn *pgx.Conn
}

func NewDBConnection(psqlLine string) (*DBConnection, error) {
	connConfig, err := pgx.ParseConnectionString(psqlLine)
	if err != nil {
		return nil, err
	}
	db, err := pgx.Connect(connConfig)
	if err != nil {
		return nil, err
	}
	return &DBConnection{db}, nil
}

func (db *DBConnection) Close() error {
	return db.conn.Close()
}

func (db *DBConnection) InitTables() error {
	query := `CREATE TABLE IF NOT EXISTS gauges (id SERIAL PRIMARY KEY, name VARCHAR(50), value double precision);`
	res, err := db.conn.Exec(query)
	if err != nil {
		return err
	}
	fmt.Println(res)
	query = `CREATE TABLE IF NOT EXISTS counters (id SERIAL PRIMARY KEY, name VARCHAR(50), value int);`
	res, err = db.conn.Exec(query)
	if err != nil {
		return err
	}
	fmt.Println(res)
	return nil
}

func (db *DBConnection) WriteMemStorage(storage *memstorage.MemStorage) error {
	query := `INSERT INTO counters (name, value) VALUES ($1, $2)`
	for k, v := range storage.Counters {
		res, err := db.conn.Exec(query, "counters", k, v)
		if err != nil {
			return err
		}
		fmt.Println(res)
	}
	query = `INSERT INTO gauges (name, value) VALUES ($1, $2)`
	for k, v := range storage.Gauges {
		res, err := db.conn.Exec(query, "gauges", k, v)
		if err != nil {
			return err
		}
		fmt.Println(res)
	}
	db.Close()
	return nil
}

func (db *DBConnection) WriteMetric(mType, mName, mVal string) error {
	var query string
	if mType == "gauge" {
		query = `INSERT INTO gauges (name, value) VALUES ($1, $2)`
	} else if mType == "counter" {
		query = `INSERT INTO counters (name, value) VALUES ($1, $2)`
	}
	res, err := db.conn.Exec(query, mName, mVal)
	if err != nil {
		fmt.Println("WRITEMETRIC: " + fmt.Sprint(err))
		return err
	}
	fmt.Println(res)
	return nil
}

func (db *DBConnection) ReadMemStorage() (*memstorage.MemStorage, error) {
	storage := memstorage.NewMemStorage()
	query := `
		SELECT name, value 
		FROM gauges g 
		WHERE id IN (
			SELECT MAX(id)
			FROM data
			WHERE name = g.name
			GROUP BY name
		)
	`
	rows, err := db.conn.Query(query)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var mName string
		var mVal float64
		err := rows.Scan(&mName, &mVal)
		if err != nil {
			return nil, err
		}
		storage.PutGauge(mName, mVal)
	}
	rows.Close()

	query = `
		SELECT name, value 
		FROM counters g 
		WHERE id IN (
			SELECT MAX(id)
			FROM data
			WHERE name = g.name
			GROUP BY name
		)
	`
	rows, err = db.conn.Query(query)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var mName string
		var mVal int64
		err := rows.Scan(&mName, &mVal)
		if err != nil {
			return nil, err
		}
		storage.PutCounter(mName, mVal)
	}
	rows.Close()

	return storage, nil
}
