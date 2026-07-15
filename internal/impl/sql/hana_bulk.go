// Copyright 2024 Redpanda Data, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package sql

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	hdbdriver "github.com/SAP/go-hdb/driver"

	"github.com/redpanda-data/benthos/v4/public/bloblang"
	"github.com/redpanda-data/benthos/v4/public/service"
)

// hanaWriter holds state for go-hdb bulk-insert/upsert operations.
// It is only created when driver == "hana".
type hanaWriter struct {
	execSQL string
	mu      sync.Mutex
}

func newHANAWriter(table string, columns []string, upsert bool) *hanaWriter {
	phs := make([]string, len(columns))
	for i := range phs {
		phs[i] = "?"
	}
	colList := strings.Join(columns, ", ")
	phList := strings.Join(phs, ", ")
	var execSQL string
	if upsert {
		execSQL = "UPSERT " + table + " (" + colList + ") VALUES (" + phList + ") WITH PRIMARY KEY"
	} else {
		execSQL = "INSERT INTO " + table + " (" + colList + ") VALUES (" + phList + ")"
	}
	return &hanaWriter{execSQL: execSQL}
}

// openHANADB opens a go-hdb connection tuned for bulk insert.
//
// We bypass sql.Open and use a DSN connector directly so we can set BulkSize
// (guarantees one MT_EXECUTE per WriteBatch call) and restore the TCP timeout
// that NewDSNConnector zeros when the DSN has no timeout= parameter.
// MaxIdleConns=0 closes the connection after each batch, avoiding HANA
// server-side post-commit state that can block the next MT_EXECUTE.
func openHANADB(dsn string) (*sql.DB, error) {
	ctr, err := hdbdriver.NewDSNConnector(dsn)
	if err != nil {
		return nil, err
	}
	ctr.SetTimeout(30 * time.Second)
	ctr.SetBulkSize(100_000)
	db := sql.OpenDB(ctr)
	db.SetMaxIdleConns(0)
	return db, nil
}

// writeBatch performs a go-hdb bulk insert/upsert for one benthos batch.
//
// mu serialises concurrent calls: concurrent MT_EXECUTE to the same table
// causes HANA row-level lock contention. (*sql.Conn).ExecContext is used
// instead of (*sql.DB).ExecContext to avoid the DB-level retry loop that
// re-invokes the callback with idx already exhausted, silently writing 0 rows.
func (h *hanaWriter) writeBatch(ctx context.Context, db *sql.DB, batch service.MessageBatch, argsMapping *bloblang.Executor) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	var argsExec *service.MessageBatchBloblangExecutor
	if argsMapping != nil {
		argsExec = batch.BloblangExecutor(argsMapping)
	}
	batchArgs := make([][]any, 0, len(batch))
	for i := range batch {
		if argsExec == nil {
			break
		}
		resMsg, err := argsExec.Query(i)
		if err != nil {
			return err
		}
		iargs, err := resMsg.AsStructured()
		if err != nil {
			return err
		}
		args, ok := iargs.([]any)
		if !ok {
			return fmt.Errorf("mapping returned non-array result: %T", iargs)
		}
		batchArgs = append(batchArgs, args)
	}

	execCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	conn, err := db.Conn(execCtx)
	if err != nil {
		return err
	}
	defer conn.Close()

	idx := 0
	_, err = conn.ExecContext(execCtx, h.execSQL, func(args []any) error {
		if idx >= len(batchArgs) {
			return hdbdriver.ErrEndOfRows
		}
		copy(args, batchArgs[idx])
		idx++
		return nil
	})
	return err
}
