/*
Copyright (c) 2022 PaddlePaddle Authors. All Rights Reserve.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package driver

import (
	"fmt"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	"gorm.io/driver/mysql"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"

	"github.com/PaddlePaddle/PaddleFlow/pkg/apiserver/models"
	"github.com/PaddlePaddle/PaddleFlow/pkg/common/config"
	"github.com/PaddlePaddle/PaddleFlow/pkg/model"
	"github.com/PaddlePaddle/PaddleFlow/pkg/storage"
)

const (
	Mysql  = "mysql"
	Sqlite = "sqlite"
	// data init for sqllite
	// default busy timeout 2 min
	dsn              = "file:paddleflow.db?cache=shared&mode=rwc&_busy_timeout=120000"
	rootUserName     = "root"
	rootUserPassword = "$2a$10$1qdSQN5wMl3FtXoxw7mKpuxBqIuP0eYXTBM9CBn5H4KubM/g5Hrb6%"
)

func InitStorage(conf *config.StorageConfig, logLevel string) error {
	driver := strings.ToLower(conf.Driver)
	gormConf := getGormConf(logLevel)
	switch driver {
	case Mysql:
		storage.DB = initMysqlDB(conf, gormConf)
	default:
		// 若配置文件没有设置，则默认使用SQLLite
		// fix table lock
		storage.DB = initSQLiteDB(gormConf)
		// set 1 connection
		conf.MaxOpenConns = new(int)
		*conf.MaxOpenConns = 1
	}

	if storage.DB == nil {
		panic(fmt.Errorf("Init database DB error\n"))
	}
	if err := setSqlDBConns(conf); err != nil {
		return err
	}

	log.Debugf("InitStorage success.dbConf:%v", conf)
	storage.InitStores(storage.DB)
	return nil
}

func InitCache(logLevel string) error {
	gormConf := getGormConf(logLevel)
	gormConf.Logger.LogMode(logger.Info)

	db, err := gorm.Open(sqlite.Open("file::memory:"), gormConf)
	if err != nil {
		log.Fatalf("init sqlite open db error: %v", err)
		return err
	}
	err = db.AutoMigrate(
		&model.NodeInfo{},
		&model.PodInfo{},
		&model.ResourceInfo{},
		&model.LabelInfo{},
	)
	if err != nil {
		log.Fatalf("init sqlite create database tables failed, error: %v", err)
		return err
	}
	sqlDB, err := db.DB()
	if err != nil {
		log.Fatalf("Get DB.DB error: %s", err.Error())
		return err
	}
	// Set max open connections to 1, because of no such table error.
	// TODO: evaluate the write performance when max open connections is 1
	sqlDB.SetMaxOpenConns(1)
	log.Debugf("InitCache with conf: %v", gormConf)
	storage.ClusterCache = db
	storage.InitClusterCaches(storage.ClusterCache)
	return nil
}

func getGormConf(logLevel string) *gorm.Config {
	gormConf := &gorm.Config{
		NamingStrategy: schema.NamingStrategy{
			TablePrefix:   "",
			SingularTable: true,
		},
		Logger: logger.Default,
	}

	if level, err := log.ParseLevel(logLevel); err != nil {
		log.Warningf("Parse log level error[%s], using logger.Default as gormLogger.", err.Error())
	} else if level == log.DebugLevel || level == log.TraceLevel {
		gormConf.Logger.LogMode(logger.Info)
	}
	return gormConf
}

func setSqlDBConns(conf *config.StorageConfig) error {
	sqlDB, err := storage.DB.DB()
	if err != nil {
		log.Fatalf("Get DB.DB error[%s]", err.Error())
		return err
	}

	if conf.MaxIdleConns == nil {
		conf.MaxIdleConns = new(int)
		*conf.MaxIdleConns = 5
	}
	sqlDB.SetMaxIdleConns(*conf.MaxIdleConns)

	if conf.MaxOpenConns == nil {
		conf.MaxOpenConns = new(int)
		*conf.MaxOpenConns = 10
	}
	sqlDB.SetMaxOpenConns(*conf.MaxOpenConns)

	if conf.ConnMaxLifetimeInHours == nil {
		conf.ConnMaxLifetimeInHours = new(int)
		*conf.ConnMaxLifetimeInHours = 1
	}
	sqlDB.SetConnMaxLifetime(time.Hour * time.Duration(*conf.ConnMaxLifetimeInHours))
	return nil
}

func InitMockDB() {
	// github.com/mattn/go-sqlite3
	db, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{
		// print sql
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		log.Fatalf("initMockDB open db error: %v", err)
	}
	if err := createDatabaseTables(db); err != nil {
		log.Fatalf("initMockDB createDatabaseTables error[%s]", err.Error())
	}
	storage.DB = db
	storage.InitStores(db)
}

func initSQLiteDB(gormConf *gorm.Config) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(dsn), gormConf)
	if err != nil {
		log.Fatalf("initSQLiteDB error[%s]", err.Error())
		return nil
	}

	if err := createDatabaseTables(db); err != nil {
		log.Fatalf("initSQLiteDB createDatabaseTables error[%s]", err.Error())
		return nil
	}
	// init root user to db, can not be modified by config file currently
	rootUser := model.User{
		UserInfo: model.UserInfo{
			Name:     rootUserName,
			Password: rootUserPassword,
		},
	}

	tx := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "name"}},
		DoUpdates: clause.AssignmentColumns([]string{"password"}),
	}).Create(&rootUser)
	if tx.Error != nil {
		log.Fatalf("init sqllite db error[%s]", tx.Error)
		return nil
	}
	// init flavour to db
	flavours := []model.Flavour{
		{
			Name: "flavour1",
			CPU:  "1",
			Mem:  "1Gi",
		},
		{
			Name:               "flavour2",
			CPU:                "4",
			Mem:                "8Gi",
			RawScalarResources: `{"nvidia.com/gpu": "1"}`,
		},
		{
			Name:               "flavour3",
			CPU:                "4",
			Mem:                "8Gi",
			RawScalarResources: `{"nvidia.com/gpu": "2"}`,
		},
	}
	tx = db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "name"}},
		DoUpdates: clause.AssignmentColumns([]string{"cpu", "mem", "scalar_resources"}),
	}).CreateInBatches(&flavours, 4)
	if tx.Error != nil {
		log.Fatalf("init sqllite db error[%s]", tx.Error)
		return nil
	}

	log.Debugf("init sqlite DB success")
	return db
}

func initMysqlDB(dbConf *config.StorageConfig, gormConf *gorm.Config) *gorm.DB {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8&parseTime=True&loc=Local",
		dbConf.User, dbConf.Password, dbConf.Host, dbConf.Port, dbConf.Database)
	db, err := gorm.Open(mysql.Open(dsn), gormConf)
	if err != nil {
		log.Fatalf("initMysqlDB error[%s]", err.Error())
		return nil
	}
	log.Debugf("init mysql DB success")
	return db
}

func createDatabaseTables(db *gorm.DB) error {
	return db.AutoMigrate(
		&model.Pipeline{},
		&model.PipelineVersion{},
		&models.Schedule{},
		&models.RunCache{},
		&model.ArtifactEvent{},
		&model.User{},
		&models.Run{},
		&models.RunJob{},
		&models.RunDag{},
		&model.Queue{},
		&model.Flavour{},
		&model.Grant{},
		&model.Job{},
		&model.JobTask{},
		&model.JobLabel{},
		&model.ClusterInfo{},
		&model.Image{},
		&model.FileSystem{},
		&model.Link{},
		&model.FSCacheConfig{},
		&model.FSCache{},
	)
}
