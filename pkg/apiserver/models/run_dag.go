/*
Copyright (c) 2021 PaddlePaddle Authors. All Rights Reserve.

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

package models

import (
	"database/sql"
	"encoding/json"
	"time"

	"gorm.io/gorm"

	"github.com/PaddlePaddle/PaddleFlow/pkg/common/database"
	"github.com/PaddlePaddle/PaddleFlow/pkg/common/logger"
	"github.com/PaddlePaddle/PaddleFlow/pkg/common/schema"
	log "github.com/sirupsen/logrus"
)

type RunDag struct {
	Pk             int64             `gorm:"primaryKey;autoIncrement;not null"  json:"-"`
	ID             string            `gorm:"type:varchar(60);not null"          json:"jobID"`
	RunID          string            `gorm:"type:varchar(60);not null"          json:"runID"`
	ParentDagID    string            `gorm:"type:varchar(60);not null"          json:"parentDagID"`
	Name           string            `gorm:"type:varchar(60);not null"          json:"name"`
	DagName        string            `gorm:"type:varchar(60);not null"          json:"dag_name"`
	Parameters     map[string]string `gorm:"-"                                  json:"parameters"`
	ParametersJson string            `gorm:"type:text;size:65535;not null"      json:"-"`
	Artifacts      schema.Artifacts  `gorm:"-"                                  json:"artifacts"`
	ArtifactsJson  string            `gorm:"type:text;size:65535;not null"      json:"-"`
	Status         schema.JobStatus  `gorm:"type:varchar(32);not null"          json:"status"`
	Message        string            `gorm:"type:text;size:65535;not null"      json:"message"`
	CreateTime     string            `gorm:"-"                                  json:"createTime"`
	ActivateTime   string            `gorm:"-"                                  json:"activateTime"`
	UpdateTime     string            `gorm:"-"                                  json:"updateTime,omitempty"`
	CreatedAt      time.Time         `                                          json:"-"`
	ActivatedAt    sql.NullTime      `                                          json:"-"`
	UpdatedAt      time.Time         `                                          json:"-"`
	DeletedAt      gorm.DeletedAt    `gorm:"index"                              json:"-"`
}

func CreateRunDag(logEntry *log.Entry, runDag *RunDag) (int64, error) {
	logEntry.Debugf("begin create run_dag model: %v", runDag)
	err := withTransaction(database.DB, func(tx *gorm.DB) error {
		result := tx.Model(&RunDag{}).Create(&runDag)
		if result.Error != nil {
			logEntry.Errorf("create run_dag failed. run_dag: %v, error: %s",
				runDag, result.Error.Error())
			return result.Error
		}
		return nil
	})
	return runDag.Pk, err
}

func UpdateRunDag(logEntry *log.Entry, pk int64, runDag RunDag) error {
	logEntry.Debugf("begin update run_dag")
	tx := database.DB.Model(&RunDag{}).Where("pk = ?", pk).Updates(runDag)
	if tx.Error != nil {
		logEntry.Errorf("update run_dag failed. pk: %v, run_dag: %v, error: %s",
			pk, runDag, tx.Error.Error())
		return tx.Error
	}
	return nil
}

func ParseRunDag(dagView *schema.DagView) RunDag {
	newParameters := map[string]string{}
	for k, v := range dagView.Parameters {
		newParameters[k] = v
	}

	return RunDag{
		ID:           dagView.DagID,
		Name:         dagView.DagName,
		ParentDagID:  dagView.ParentDagID,
		Parameters:   newParameters,
		Artifacts:    dagView.Artifacts,
		Status:       dagView.Status,
		Message:      dagView.Message,
		ActivateTime: dagView.StartTime,
	}
}

func (rj *RunDag) Encode() error {
	artifactJson, err := json.Marshal(rj.Artifacts)
	if err != nil {
		logger.Logger().Errorf("encode run job artifact failed. error:%v", err)
		return err
	}
	rj.ArtifactsJson = string(artifactJson)

	parametersJson, err := json.Marshal(rj.Parameters)
	if err != nil {
		logger.Logger().Errorf("encode run job parameters failed. error:%v", err)
		return err
	}
	rj.ParametersJson = string(parametersJson)

	if rj.ActivateTime != "" {
		activatedAt := sql.NullTime{}
		activatedAt.Time, err = time.ParseInLocation("2006-01-02 15:04:05", rj.ActivateTime, time.Local)
		activatedAt.Valid = true
		if err != nil {
			logger.Logger().Errorf("encode run job activateTime failed. error: %v", err)
			return err
		}
		rj.ActivatedAt = activatedAt
	}

	return nil
}
