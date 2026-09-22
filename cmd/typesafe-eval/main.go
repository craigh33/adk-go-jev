package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/craigh33/adk-go-typesafe/evaluation"
	"github.com/craigh33/adk-go-typesafe/typesafe"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	path := flag.String("dataset", "", "JSON dataset to evaluate")
	model := flag.String("model", "", "Override the dataset's TypeSafe model")
	threshold := flag.Float64("min-confidence", -1, "Override the dataset's confidence threshold (0 to 1)")
	timeout := flag.Duration("timeout", 5*time.Minute, "Deadline for the full evaluation")
	flag.Parse()
	if *path == "" || *timeout <= 0 {
		return errors.New("provide -dataset and a positive -timeout")
	}
	file, err := os.Open(*path)
	if err != nil {
		return err
	}
	defer file.Close()
	dataset, err := evaluation.Load(file)
	if err != nil {
		return err
	}
	if *model != "" {
		dataset.Model = *model
	}
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "min-confidence" {
			dataset.MinConfidence = *threshold
		}
	})
	client, err := typesafe.New(nil)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()
	report, runErr := evaluation.Run(ctx, client, *dataset)
	if report != nil {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(report); err != nil {
			return err
		}
	}
	if runErr != nil {
		return runErr
	}
	if report.Errors > 0 {
		return fmt.Errorf("%d evaluation cases failed", report.Errors)
	}
	return nil
}
