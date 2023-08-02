package filerw

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	"github.com/kishenkoilya/metricsalerts/internal/memstorage"
)

type Producer struct {
	file *os.File
	// добавляем Writer в Producer
	writer *bufio.Writer
}

type Consumer struct {
	file *os.File
	// заменяем Reader на Scanner
	scanner *bufio.Scanner
}

type Metric struct {
	ID    string `json:"id"`
	MType string `json:"type"`
	MVal  string `json:"value"`
}

func NewProducer(filename string, trunc bool) (*Producer, error) {
	var file *os.File
	var err error
	if trunc {
		fmt.Println(trunc)
		file, err = os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_APPEND|os.O_TRUNC, 0666)
	} else {
		fmt.Println(trunc)
		file, err = os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0666)
	}
	if err != nil {
		return nil, err
	}

	return &Producer{
		file: file,
		// создаём новый Writer
		writer: bufio.NewWriter(file),
	}, nil
}

func (p *Producer) WriteMemStorage(storage *memstorage.MemStorage) error {
	for k, v := range storage.Counters {
		metric := Metric{ID: k, MType: "counter", MVal: fmt.Sprint(v)}
		err := p.WriteMetric(&metric)
		if err != nil {
			return err
		}
	}
	for k, v := range storage.Gauges {
		metric := Metric{ID: k, MType: "gauge", MVal: fmt.Sprint(v)}
		err := p.WriteMetric(&metric)
		if err != nil {
			return err
		}
	}
	p.Close()
	return nil
}

func (p *Producer) WriteMetric(metric *Metric) error {
	data, err := json.Marshal(&metric)
	if err != nil {
		return err
	}

	// записываем событие в буфер
	if _, err := p.writer.Write(data); err != nil {
		return err
	}

	// добавляем перенос строки
	if err := p.writer.WriteByte('\n'); err != nil {
		return err
	}

	// записываем буфер в файл
	return p.writer.Flush()
}

func (p *Producer) Close() error {
	// закрываем файл
	return p.file.Close()
}

func NewConsumer(filename string) (*Consumer, error) {
	file, err := os.OpenFile(filename, os.O_RDONLY|os.O_CREATE, 0666)
	if err != nil {
		return nil, err
	}

	return &Consumer{
		file: file,
		// создаём новый scanner
		scanner: bufio.NewScanner(file),
	}, nil
}

func (c *Consumer) ReadMemStorage() (*memstorage.MemStorage, error) {
	storage := memstorage.NewMemStorage()
	var err error
	metric := &Metric{}
	for err == nil {
		metric, err = c.ReadMetric()
		if metric == nil {
			break
		}
		if metric.MType == "counter" {
			val, err := strconv.ParseInt(metric.MVal, 0, 64)
			if err != nil {
				return memstorage.NewMemStorage(), err
			}
			storage.PutCounter(metric.ID, val)
		}
		if metric.MType == "gauge" {
			val, err := strconv.ParseFloat(metric.MVal, 64)
			if err != nil {
				return memstorage.NewMemStorage(), err
			}
			storage.PutGauge(metric.ID, val)
		}
	}
	return storage, nil
}

func (c *Consumer) ReadMetric() (*Metric, error) {
	// одиночное сканирование до следующей строки
	if !c.scanner.Scan() {
		return nil, c.scanner.Err()
	}
	// читаем данные из scanner
	data := c.scanner.Bytes()

	metric := Metric{}
	err := json.Unmarshal(data, &metric)
	if err != nil {
		return nil, err
	}

	return &metric, nil
}

func (c *Consumer) Close() error {
	return c.file.Close()
}
