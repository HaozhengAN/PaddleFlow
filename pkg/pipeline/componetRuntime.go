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

package pipeline

import (
	"context"
	"fmt"

	"github.com/sirupsen/logrus"

	"github.com/PaddlePaddle/PaddleFlow/pkg/apiserver/common"
	"github.com/PaddlePaddle/PaddleFlow/pkg/common/schema"
	. "github.com/PaddlePaddle/PaddleFlow/pkg/pipeline/common"
)

type RuntimeStatus = schema.JobStatus

var (
	StatusRuntimeInit        RuntimeStatus = schema.StatusJobInit
	StatusRunttimePending    RuntimeStatus = schema.StatusJobPending
	StatusRuntimeRunning     RuntimeStatus = schema.StatusJobRunning
	StatusRuntimeFailed      RuntimeStatus = schema.StatusJobFailed
	StatusRuntimeSucceeded   RuntimeStatus = schema.StatusJobSucceeded
	StatusRuntimeTerminating RuntimeStatus = schema.StatusJobTerminating
	StatusRuntimeTerminated  RuntimeStatus = schema.StatusJobTerminated
	StatusRuntimeCancelled   RuntimeStatus = schema.StatusJobCancelled
	StatusRuntimeSkipped     RuntimeStatus = schema.StatusJobSkipped
)

// 管理并发信息
type parallelismManager struct {
	ch chan struct{}
}

func NewParallelismManager(parallelism int) *parallelismManager {
	return &parallelismManager{
		ch: make(chan struct{}, parallelism),
	}
}

func (pm *parallelismManager) increase() {
	pm.ch <- struct{}{}
}

func (pm *parallelismManager) decrease() {
	<-pm.ch
}

type componentRuntime interface {
	isSucceeded() bool
	isDone() bool
	isFailed() bool
	isCancelled() bool
	isSkipped() bool
	isTerminated() bool

	getComponent() schema.Component
	getFullName() string
}

// Run 的相关配置，其信息来源有以下几种:
// 1. workflowSource
// 2. 用户创建 Run 的请求体中 除 1 外的其余信息
// 3. Apiserver 或者 Parser 动态生成的信息，如 runID 等
type runConfig struct {
	// 1. workflowSource 中的信息
	*schema.WorkflowSource

	// 2. 来自于请求体中的信息
	fsID     string
	fsName   string
	userName string

	// pipelineID or yamlPath or md5sum of yamlRaw
	pplSource string

	// 3. 由 ApiServer 或者 Parser 动态生成的信息
	runID  string
	logger *logrus.Entry
	*parallelismManager

	// 用于与 APIServer 同步信息
	callbacks WorkflowCallbacks
}

func NewRunConfig(workflowSource *schema.WorkflowSource, fsID, fsName, userName, runID string, logger *logrus.Entry,
	callbacks WorkflowCallbacks, pplSource string) *runConfig {
	return &runConfig{
		WorkflowSource: workflowSource,

		fsID:      fsID,
		fsName:    fsName,
		userName:  userName,
		pplSource: pplSource,

		runID:              runID,
		logger:             logger,
		callbacks:          callbacks,
		parallelismManager: NewParallelismManager(workflowSource.Parallelism),
	}
}

// stepRuntime 和 DagRuntime 的基类
type baseComponentRuntime struct {
	component schema.Component

	// 类似根目录，由其所有祖先组件名加上自身名字组成，名字与名字之间以"." 分隔
	CompoentFullName string

	// runtime 的名字，由 componentFullName 和 seq 组成
	name string

	// 表明节点的第几次运行， 从 0 开始计算
	seq int

	// runtime 在数据库的主键值，方便在存库是使用，应该由 apiserver 的回调函数返回，不应该自行设置
	pk int64

	// 用于监听终止信号的上下文
	ctx context.Context

	// 用于监听 failureOptions 信号的上下文
	failureOpitonsCtx context.Context

	// 监听子节点事件的 channel
	receiveEventChildren chan WorkflowEvent

	// 将 event 同步至父节点的 channel
	sendEventToParent chan<- WorkflowEvent

	status RuntimeStatus

	// 是否处于终态
	done bool

	// run 级别的相关配置
	*runConfig

	// 用于替换 节点内部的引用模板
	*innerSolver

	// 系统环境变量的值
	sysParams map[string]string

	// 父节点ID
	parentDagID string
}

func NewBaseComponentRuntime(fullname string, component schema.Component, seq int, ctx context.Context, failureOpitonsCtx context.Context,
	eventChannel chan<- WorkflowEvent, config *runConfig, parentDagID string) *baseComponentRuntime {

	cr := &baseComponentRuntime{
		name:                 fmt.Sprintf("%s-%d", fullname, seq),
		CompoentFullName:     fullname,
		component:            component,
		seq:                  seq,
		ctx:                  ctx,
		sendEventToParent:    eventChannel,
		receiveEventChildren: make(chan WorkflowEvent),
		runConfig:            config,
		parentDagID:          parentDagID,
		failureOpitonsCtx:    failureOpitonsCtx,
	}

	isv := NewInnerSolver(component, fullname, config)
	cr.innerSolver = isv
	cr.setSysParams()
	return cr
}

// 判断当前节点是否被 disabled
func (crt *baseComponentRuntime) isDisabled() bool {
	for _, fullName := range crt.GetDisabled() {
		if fullName == crt.CompoentFullName {
			return true
		}
	}
	return false
}

func (crt *baseComponentRuntime) isSucceeded() bool {
	return crt.status == StatusRuntimeSucceeded
}

func (crt *baseComponentRuntime) isCancelled() bool {
	return crt.status == StatusRuntimeCancelled
}

func (crt *baseComponentRuntime) isFailed() bool {
	return crt.status == StatusRuntimeFailed
}

func (crt *baseComponentRuntime) isSkipped() bool {
	return crt.status == StatusRuntimeSkipped
}

func (crt *baseComponentRuntime) isTerminating() bool {
	return crt.status == StatusRuntimeTerminating
}

func (crt *baseComponentRuntime) isTerminated() bool {
	return crt.status == StatusRuntimeTerminated
}

// 判断当次运行是否已经处于终态
func (crt *baseComponentRuntime) isDone() bool {
	return crt.done
}

func (crt *baseComponentRuntime) getComponent() schema.Component {
	return crt.component
}

// 更新节点状态
func (crt *baseComponentRuntime) updateStatus(status RuntimeStatus) error {
	if crt.done {
		err := fmt.Errorf("cannot update the status of runtime[%s] for node[%s]，because the status of it is [%s]",
			crt.component.GetName(), crt.CompoentFullName, crt.status)
		crt.logger.Errorln(err.Error())
		return err
	}

	crt.status = status

	if crt.status == StatusRuntimeCancelled || crt.status == StatusRuntimeFailed || crt.status == StatusRuntimeSucceeded || crt.status == StatusRuntimeSkipped {
		crt.done = true
	}
	return nil
}

// 获取当次运行时循环参数的值
func (crt *baseComponentRuntime) getPFLoopArgument() (interface{}, error) {
	err := crt.innerSolver.resolveLoopArugment()
	if err != nil {
		return nil, err
	}

	if crt.component.GetLoopArgument() == nil {
		return nil, nil
	}
	if len(crt.component.GetLoopArgument().([]interface{})) < crt.seq {
		err := fmt.Errorf("inner error: the index of loop_argumetn is out of range")
		return nil, err
	}

	return crt.component.GetLoopArgument().([]interface{})[crt.seq], nil
}

// 获取系统变量
/*
var SysParamNameList []string = []string{
	SysParamNamePFRunID,
	SysParamNamePFFsID,
	SysParamNamePFJobID,
	SysParamNamePFStepName,
	SysParamNamePFFsName,
	SysParamNamePFUserID,
	SysParamNamePFUserName,
	SysParamNamePFLoopArgument,
}
*/
func (crt *baseComponentRuntime) setSysParams() error {
	crt.sysParams = map[string]string{
		SysParamNamePFRunID:    crt.runID,
		SysParamNamePFFsID:     crt.fsID,
		SysParamNamePFFsName:   crt.fsName,
		SysParamNamePFStepName: crt.component.GetName(),
		SysParamNamePFUserName: crt.userName,
	}

	pfLoopArugment, err := crt.getPFLoopArgument()
	if err != nil {
		return err
	}

	if pfLoopArugment == nil {
		crt.sysParams[SysParamNamePFLoopArgument] = ""
	} else {
		crt.sysParams[SysParamNamePFLoopArgument] = fmt.Sprintf("%v", pfLoopArugment)
	}

	return nil
}

func (crt *baseComponentRuntime) CalculateCondition() (bool, error) {
	crt.resolveCondition()
	cc := NewConditionCalculator(crt.component.GetCondition())
	return cc.calculate()
}

func (crt *baseComponentRuntime) syncToApiServerAndParent(wv WfEventValue, view schema.ComponentView, msg string) {
	extra := map[string]interface{}{
		common.WfEventKeyRunID:         crt.runID,
		common.WfEventKeyPK:            crt.pk,
		common.WfEventKeyStatus:        crt.status,
		common.WfEventKeyView:          view,
		common.WfEventKeyComponentName: crt.component.GetName(),
	}

	event := NewWorkflowEvent(wv, msg, extra)

	// 调用回调函数，将信息同步至 apiserver
	crt.callback(event)

	// 将事件冒泡给父节点
	crt.sendEventToParent <- *event
}

func (crt *baseComponentRuntime) callback(event *WorkflowEvent) {
	for i := 0; i < 3; i++ {
		crt.logger.Infof("callback event [%+v]", event)
		if pk, success := crt.callbacks.UpdateRuntimeCb(crt.runID, event); success {
			crt.pk = pk
			break
		}
	}
}

func (crt *baseComponentRuntime) getFullName() string {
	return crt.CompoentFullName
}
