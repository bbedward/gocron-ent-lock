# Gocron-Ent-Lock

[![golangci-lint](https://github.com/bbedward/gocron-ent-lock/actions/workflows/go_test.yml/badge.svg)](https://github.com/bbedward/gocron-ent-lock/actions/workflows/go_test.yml)
![Go Report Card](https://goreportcard.com/badge/github.com/bbedward/gocron-ent-lock)
[![Go Doc](https://godoc.org/github.com/bbedward/gocron-ent-lock?status.svg)](https://pkg.go.dev/github.com/bbedward/gocron-ent-lock)

A gocron locker implementation using Ent

## ⬇️ Install

```bash
go get github.com/bbedward/gocron-ent-lock/v2
```

## 📋 Usage

Here is an example usage that would be deployed in multiple instances

```go
package main

import (
    "context"
    "database/sql"
    "fmt"
    "time"

    "github.com/go-co-op/gocron/v2"
    entlock "github.com/bbedward/gocron-ent-lock/v2"
    "github.com/bbedward/gocron-ent-lock/v2/ent"
    _ "github.com/lib/pq"
)

func main() {
    // Connect to database
    db, err := sql.Open("postgres", "postgres://user:pass@localhost/dbname?sslmode=disable")
    if err != nil {
        panic(err)
    }
    defer db.Close()

    // Create Ent client
    client := ent.NewClient(ent.Driver(ent.NewDriver("postgres", db)))
    defer client.Close()

    ctx := context.Background()

    // Run the auto migration tool to create the schema
    if err := client.Schema.Create(ctx); err != nil {
        panic(fmt.Sprintf("failed creating schema resources: %v", err))
    }

    worker := "instance-1" // name of this instance to be used to know which instance run the job
    locker, err := entlock.NewEntLocker(client, worker)
    if err != nil {
        panic(err)
    }
    defer locker.Close()

    s, err := gocron.NewScheduler(gocron.WithDistributedLocker(locker))
    if err != nil {
        panic(err)
    }

    f := func() {
        // task to do
        fmt.Println("call 1s")
    }

    _, err = s.NewJob(gocron.DurationJob(1*time.Second), gocron.NewTask(f), gocron.WithName("unique_name"))
    if err != nil {
        // handle the error
    }

    s.Start()
}
```

To check a real use case example, check [examples](./examples).

## Prerequisites

- The table `cron_job_locks` needs to exist in the database. You can create it using Ent's schema migration: `client.Schema.Create(ctx)`
- In order to uniquely identify the job, the locker uses the unique combination of `job name + timestamp` (by default with precision to seconds). Check [JobIdentifier](#jobidentifier) for more info.

## 💡 Features

### JobIdentifier

Ent Lock tries to lock the access to a job by uniquely identify the job. The default implementation to uniquely identify the job is using the following combination [`job name and timestamp`](./ent_lock_options.go).

#### JobIdentifier Timestamp Precision

By default, the timestamp precision is in **seconds**, meaning that if a job named `myJob` is executed at `2025-01-01 10:11:12 15:16:17.000`, the resulting job identifier will be the combination of `myJob` and `2025-01-01 10:11:12`.

- It is possible to change the precision with [`WithDefaultJobIdentifier(newPrecision)`](./ent_lock_options.go), e.g. `WithDefaultJobIdentifier(time.Hour)`
- It is also possible to completely override the way the job identifier is created with the [`WithJobIdentifier()`](./ent_lock_options.go) option.

To see these two options in action, check the test [TestJobReturningExceptionWhenUnique](./ent_lock_test.go)

### Removing Old Entries

Ent Lock also removes old entries stored in the database. You can configure the time interval, and the time to live with the following options:

- `WithTTL`
- `WithCleanInterval`
