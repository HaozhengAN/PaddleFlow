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

package metrics

import (
	"strings"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	MetricJobTime    = "pf_metric_job_time"
	MetricQueueInfo  = "pf_metric_queue_info"
	MetricJobGPUInfo = "pf_metric_job_gpu_info"

	MetricPipelineRequest  = "pf_pipeline_request"
	MetricPipelineResponse = "pf_pipeline_response"

	MetricApiDuration = "pf_api_duration_millisecond"
	MetricRunStage    = "pf_run_stage"
)

func toHelp(name string) string {
	return strings.ReplaceAll(name, "_", " ")
}

const (
	JobIDLabel          = "jobID"
	GpuIdxLabel         = "id"
	StatusLabel         = "status"
	QueueIDLabel        = "queueID"
	FinishedStatusLabel = "finishedStatus"
	QueueNameLabel      = "queueName"
	UserNameLabel       = "userName"
	ResourceLabel       = "resource"
	TypeLabel           = "type"
	BaiduGpuIndexLabel  = "baidu_com_gpu_idx"

	ApiNameLabel       = "apiName"
	RequestMethodLabel = "method"
	ResponseCodeLabel  = "code"
	RunIDLabel         = "runID"
	RunStageLabel      = "runStage"
	RunStepNameLabel   = "runStepName"
	RunJobIDLabel      = "runJobID"
)

var APiDurationSummary = prometheus.NewSummaryVec(
	prometheus.SummaryOpts{
		Name:       "MetricApiDuration",
		Help:       toHelp(MetricApiDuration),
		Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001, 1: 0},
	},
	[]string{ApiNameLabel, RequestMethodLabel, ResponseCodeLabel})
