package psqlinteraction

import (
	"fmt"

	"github.com/jackc/pgx"
	"github.com/kishenkoilya/metricsalerts/internal/memstorage"
)

type RetryFunc func() (interface{}, error)

func PingPSQL(psqlLine string) RetryFunc {
	return func() (interface{}, error) {
		connConfig, err := pgx.ParseConnectionString(psqlLine)
		if err != nil {
			return nil, err
		}
		db, err := pgx.Connect(connConfig)
		if err != nil {
			return nil, err
		}
		defer db.Close()
		return nil, nil
	}
}

type DBConnection struct {
	conn *pgx.Conn
}

func NewDBConnection(psqlLine string) RetryFunc {
	return func() (interface{}, error) {
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
}

func (db *DBConnection) Close() error {
	return db.conn.Close()
}

func (db *DBConnection) InitTables() RetryFunc {
	return func() (interface{}, error) {
		query := `CREATE TABLE IF NOT EXISTS gauges (id SERIAL PRIMARY KEY, name VARCHAR(50), value double precision);`
		res, err := db.conn.Exec(query)
		if err != nil {
			return nil, err
		}
		fmt.Println(res)
		query = `CREATE TABLE IF NOT EXISTS counters (id SERIAL PRIMARY KEY, name VARCHAR(50), value bigint);`
		res, err = db.conn.Exec(query)
		if err != nil {
			return nil, err
		}
		fmt.Println(res)
		return nil, nil
	}
}

func (db *DBConnection) WriteMemStorage(storage *memstorage.MemStorage) RetryFunc {
	return func() (interface{}, error) {
		tx, err := db.conn.Begin()
		if err != nil {
			return nil, err
		}
		counters := storage.GetCounters()
		gauges := storage.GetGauges()

		for k, v := range counters {
			_, err := tx.Exec("INSERT INTO counters (name, value) VALUES($1,$2)", k, v)
			if err != nil {
				tx.Rollback()
				return nil, err
			}
		}
		for k, v := range gauges {
			_, err := tx.Exec("INSERT INTO gauges (name, value) VALUES($1,$2)", k, v)
			if err != nil {
				tx.Rollback()
				return nil, err
			}
		}
		err = tx.Commit()
		if err != nil {
			db.Close()
			return nil, err
		}
		db.Close()
		return nil, nil
	}
}

func (db *DBConnection) WriteMetric(mType, mName, mVal string) RetryFunc {
	return func() (interface{}, error) {
		var query string
		if mType == "gauge" {
			query = `INSERT INTO gauges (name, value) VALUES ($1, $2)`
		} else if mType == "counter" {
			query = `INSERT INTO counters (name, value) VALUES ($1, $2)`
		}
		res, err := db.conn.Exec(query, mName, mVal)
		if err != nil {
			fmt.Println("WRITEMETRIC: " + fmt.Sprint(err))
			return nil, err
		}
		fmt.Println(res)
		return nil, nil
	}
}

func (db *DBConnection) WriteMetrics(metrics *[]memstorage.Metrics) RetryFunc {
	return func() (interface{}, error) {
		tx, err := db.conn.Begin()
		if err != nil {
			return nil, err
		}
		for _, v := range *metrics {
			if v.Value == nil {
				_, err := tx.Exec(
					"INSERT INTO counters (name, value)"+
						" VALUES($1,$2)", v.ID, *v.Delta)
				if err != nil {
					tx.Rollback()
					return nil, err
				}
			} else {
				_, err := tx.Exec(
					"INSERT INTO gauges (name, value)"+
						" VALUES($1,$2)", v.ID, *v.Value)
				if err != nil {
					tx.Rollback()
					return nil, err
				}
			}
		}
		return nil, tx.Commit()
	}
}

func (db *DBConnection) ReadMemStorage() RetryFunc {
	return func() (interface{}, error) {
		storage := memstorage.NewMemStorage()
		query := `
		SELECT name, value 
		FROM gauges
		WHERE id IN (
			SELECT MAX(id)
			FROM gauges
			WHERE name = gauges.name
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
		FROM counters 
		WHERE id IN (
			SELECT MAX(id)
			FROM counters
			WHERE name = counters.name
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
}
