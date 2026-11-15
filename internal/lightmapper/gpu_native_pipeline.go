//go:build !js

package lightmapper

import (
	"errors"
	"fmt"
	"strings"

	"github.com/gogpu/gputypes"
	wgpu "github.com/karty-game/karty-webgpu/internal/webgpu"
)

const (
	stageGenerate = iota
	stageIntersect
	stageSelect
	stageShadow
	stageExact
	stageContinue
	stageReduce
	stageResolve
	stageReconstruct
	stageReset
	stageDispatch
	stageCompact
	stageAdvance
	stageSerialize
)
const (
	layoutStandard = iota
	layoutQueue
	layoutTrace
)

func nativeStageName(index int) string {
	return [...]string{
		"lm_generate", "lm_intersect", "lm_select", "lm_shadow", "lm_exact", "lm_continue", "lm_reduce", "lm_resolve",
		"lm_reconstruct", "lm_reset", "lm_dispatch", "lm_compact", "lm_advance", "lm_serialize",
	}[index]
}

func stageLayout(stage int) int {
	switch stage {
	case stageReset, stageDispatch:
		return layoutQueue
	case stageIntersect, stageShadow, stageExact, stageAdvance:
		return layoutTrace
	default:
		return layoutStandard
	}
}
func nativeSpecializeShader(source string, trace bool) (string, error) {
	// The binding does not expose pipeline constants. Keep the ABI defaults
	// explicit and specialize only our own diagnostic override before compilation.
	for _, declaration := range []string{"override lmPoolABI:u32=1u;", "override lmSceneABI:u32=5u;", "override lmProfileRays:bool=false;"} {
		if strings.Count(source, declaration) != 1 {
			return "", ErrInvalid
		}
	}

	if trace {
		source = strings.Replace(source, "override lmProfileRays:bool=false;", "override lmProfileRays:bool=true;", 1)
	}

	return source, nil
}
func (executor *nativeGPUExecutor) compile(source string) error {
	source, err := nativeSpecializeShader(source, executor.traceRays)
	if err != nil {
		return err
	}

	if err = executor.checked(func() error {
		executor.shader, err = executor.gpu.CreateShaderModuleWGSL(source)

		return err
	}); err != nil {
		return err
	}

	return executor.checked(func() error {
		if err := executor.compileLayouts(); err != nil {
			return err
		}

		for index := range executor.stages {
			name := nativeStageName(index)

			if err := executor.ctx.Err(); err != nil {
				return err
			}

			executor.stages[index], err = executor.gpu.CreateComputePipeline(&wgpu.ComputePipelineDescriptor{
				Label: name, Layout: executor.pipelineLayouts[stageLayout(index)], Module: executor.shader, EntryPoint: name,
			})
			if err != nil {
				return err
			}
		}

		return nil
	})
}
func nativeLayoutBinding(index uint32, kind gputypes.BufferBindingType, minimum uint64) wgpu.BindGroupLayoutEntry {
	return wgpu.BindGroupLayoutEntry{Binding: index, Visibility: gputypes.ShaderStageCompute,
		Buffer: &wgpu.BufferBindingLayout{Type: kind, MinBindingSize: minimum}}
}
func (executor *nativeGPUExecutor) compileLayouts() error {
	standard := []wgpu.BindGroupLayoutEntry{
		nativeLayoutBinding(0, gputypes.BufferBindingTypeUniform, 64),
		nativeLayoutBinding(1, gputypes.BufferBindingTypeReadOnlyStorage, 4),
		nativeLayoutBinding(2, gputypes.BufferBindingTypeReadOnlyStorage, 4),
		nativeLayoutBinding(3, gputypes.BufferBindingTypeStorage, gpuPathMinimumBytes),
		nativeLayoutBinding(4, gputypes.BufferBindingTypeStorage, 4),
	}

	entries := [][]wgpu.BindGroupLayoutEntry{standard,
		{standard[0], standard[3], nativeLayoutBinding(5, gputypes.BufferBindingTypeStorage, 12)},
		{standard[0], standard[1], standard[2], standard[3], nativeLayoutBinding(6, gputypes.BufferBindingTypeReadOnlyStorage, 16)},
	}
	for index, bindings := range entries {
		var err error

		executor.layouts[index], err = executor.gpu.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{Entries: bindings})
		if err != nil {
			return err
		}

		executor.pipelineLayouts[index], err = executor.gpu.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{
			BindGroupLayouts: []*wgpu.BindGroupLayout{executor.layouts[index]},
		})
		if err != nil {
			return err
		}
	}

	return nil
}
func (executor *nativeGPUExecutor) checked(operation func() error) error {
	// Capture validation and allocation failures. Always pop every scope, including
	// failed operations; wgpu-native v29 does not implement the Internal filter.
	filters := [...]wgpu.ErrorFilter{wgpu.ErrorFilterValidation, wgpu.ErrorFilterOutOfMemory}
	for _, filter := range filters {
		executor.gpu.PushErrorScope(filter)
	}

	err := operation()

	for range filters {
		kind, message, popErr := executor.gpu.PopErrorScopeAsync(executor.instance)

		err = errors.Join(err, popErr)
		if kind != wgpu.ErrorTypeNoError {
			err = errors.Join(err, fmt.Errorf("offline WebGPU: %s: %w", message, errGPUExecution))
		}
	}

	return err
}
