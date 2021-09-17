// Copyright 2021 PingCAP, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/pingcap/errors"
	"github.com/pingcap/log"
	"github.com/pingcap/tidb-tools/pkg/utils"
	"github.com/pingcap/tidb-tools/sync_diff_inspector/config"
	sync_utils "github.com/pingcap/tidb-tools/sync_diff_inspector/utils"
	"go.uber.org/zap"
)

func main() {
	cfg := config.NewConfig()
	err := cfg.Parse(os.Args[1:])
	switch errors.Cause(err) {
	case nil:
	case flag.ErrHelp:
		os.Exit(0)
	default:
		log.Error("parse cmd flags", zap.Error(err))
		os.Exit(2)
	}

	if cfg.PrintVersion {
		fmt.Printf("version: \n%s", utils.GetRawInfo("sync_diff_inspector"))
		return
	}

	conf := new(log.Config)
	conf.Level = cfg.LogLevel

	conf.File.Filename = filepath.Join(cfg.Task.OutputDir, "sync_diff.log")
	lg, p, e := log.InitLogger(conf)
	if e != nil {
		log.Error("Log init failed!", zap.String("error", e.Error()))
		return
	}
	log.ReplaceGlobals(lg, p)

	utils.PrintInfo("sync_diff_inspector")
	err = startPProf(cfg)

	// Initial config
	cfg.Init()

	ok := cfg.CheckConfig()
	if !ok {
		log.Error("there is something wrong with your config, please check it!")
		return
	}

	log.Info("", zap.Stringer("config", cfg))

	ctx := context.Background()
	if !checkSyncState(ctx, cfg) {
		log.Warn("check failed!!!")
		os.Exit(1)
	}
	log.Info("check pass!!!")
}

func checkSyncState(ctx context.Context, cfg *config.Config) bool {
	beginTime := time.Now()
	defer func() {
		log.Info("check data finished", zap.Duration("cost", time.Since(beginTime)))
	}()

	d, err := NewDiff(ctx, cfg)
	if err != nil {
		log.Fatal("failed to initialize diff process", zap.Error(err))
	}
	defer d.Close()

	if !d.ignoreStructCheck {
		err = d.StructEqual(ctx)
		if err != nil {
			log.Fatal("failed to check structure difference", zap.Error(err))
		}
	}
	if !d.ignoreDataCheck {
		err = d.Equal(ctx)
		if err != nil {
			log.Fatal("failed to check data difference", zap.Error(err))
		}
	}
	return d.PrintSummary(ctx, &cfg.Task)
}

func startPProf(cfg *config.Config) error {
	// Initialize the pprof server.
	if cfg.StatusAddr != "" {
		return sync_utils.StartPProfListener(cfg.StatusAddr)
	}
	sync_utils.StartDynamicPProfListener()
	return nil
}
