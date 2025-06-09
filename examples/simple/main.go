package main

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	entlock "github.com/bbedward/gocron-ent-lock/v2"
	"github.com/bbedward/gocron-ent-lock/v2/ent"
	"github.com/go-co-op/gocron/v2"
	_ "github.com/lib/pq"
)

func jobFunc() {
	fmt.Println("job func")
}

func main() {
	// In this example we use an in-memory SQLite database
	// In production, you would use PostgreSQL, MySQL, or another supported database
	db, err := sql.Open("postgres", "postgres://postgres:password@localhost/test?sslmode=disable")
	if err != nil {
		panic(err)
	}
	defer db.Close()

	client := ent.NewClient(ent.Driver(ent.NewDriver("postgres", db)))
	defer client.Close()

	ctx := context.Background()

	// Run the auto migration tool to create the schema
	if err := client.Schema.Create(ctx); err != nil {
		panic(fmt.Sprintf("failed creating schema resources: %v", err))
	}

	worker := "example"
	locker, err := entlock.NewEntLocker(client, worker)
	if err != nil {
		panic(err)
	}
	defer locker.Close()

	s, err := gocron.NewScheduler(gocron.WithDistributedLocker(locker))
	if err != nil {
		panic(err)
	}

	_, err = s.NewJob(gocron.DurationJob(1*time.Second), gocron.NewTask(jobFunc), gocron.WithName("unique_name"))
	if err != nil {
		panic(err)
	}
	_, err = s.NewJob(gocron.DurationJob(1*time.Second), gocron.NewTask(jobFunc), gocron.WithName("unique_name"))
	if err != nil {
		panic(err)
	}

	s.Start()
	time.Sleep(4 * time.Second)

	err = s.Shutdown()
	if err != nil {
		panic(err)
	}
}
