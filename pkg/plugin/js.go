package plugin

import (
	"errors"
	"path/filepath"
	"sync"

	"github.com/robertkrimen/otto"
	"github.com/stashapp/stash/pkg/plugin/common"
	"github.com/stashapp/stash/pkg/plugin/js"
)

var errStop = errors.New("stop")

type jsTaskBuilder struct{}

func (*jsTaskBuilder) build(task pluginTask) Task {
	return &jsPluginTask{
		pluginTask: task,
	}
}

type jsPluginTask struct {
	pluginTask

	started   bool
	waitGroup sync.WaitGroup
	vm        *otto.Otto
}

func throw(vm *otto.Otto, str string) {
	value, _ := vm.Call("new Error", nil, str)
	panic(value)
}

func (t *jsPluginTask) onError(err error) {
	errString := err.Error()
	t.result = &common.PluginOutput{
		Error: &errString,
	}
}

func (t *jsPluginTask) makeOutput(o otto.Value) {
	t.result = &common.PluginOutput{}

	asObj := o.Object()
	if asObj == nil {
		return
	}

	t.result.Output, _ = asObj.Get("Output")
	err, _ := asObj.Get("Error")
	if !err.IsUndefined() {
		errStr := err.String()
		t.result.Error = &errStr
	}
}

func (t *jsPluginTask) Start() error {
	if t.started {
		return errors.New("task already started")
	}

	t.started = true

	if len(t.plugin.Exec) == 0 {
		return errors.New("no script specified in exec")
	}

	scriptFile := t.plugin.Exec[0]

	t.vm = otto.New()
	pluginPath := t.plugin.getConfigPath()
	script, err := t.vm.Compile(filepath.Join(pluginPath, scriptFile), nil)
	if err != nil {
		return err
	}

	input := t.buildPluginInput()

	t.vm.Set("input", input)
	js.AddLogAPI(t.vm, t.progress)
	js.AddUtilAPI(t.vm)
	js.AddGQLAPI(t.vm, t.gqlHandler)

	t.vm.Interrupt = make(chan func(), 1)

	t.waitGroup.Add(1)

	go func() {
		defer func() {
			t.waitGroup.Done()

			if caught := recover(); caught != nil {
				if caught == errStop {
					// TODO - log this
					return
				}
			}
		}()

		output, err := t.vm.Run(script)

		if err != nil {
			t.onError(err)
		} else {
			t.makeOutput(output)
		}
	}()

	return nil
}

func (t *jsPluginTask) Wait() {
	t.waitGroup.Wait()
}

func (t *jsPluginTask) Stop() error {
	// TODO - need another way of doing this that doesn't require panic
	t.vm.Interrupt <- func() {
		panic(errStop)
	}
	return nil
}
