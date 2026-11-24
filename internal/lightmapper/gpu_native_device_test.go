//go:build !js

package lightmapper

import (
	"errors"
	"runtime"
	"strings"
	"testing"

	wgpu "github.com/karty-game/ashura/internal/webgpu"
)

func TestNativeGoAdapterAdmissionRequiresHardwareOrExplicitVulkanCPU(t *testing.T) {
	t.Parallel()

	for _, entry := range []struct {
		info      wgpu.AdapterInfoGo
		ci, valid bool
	}{
		{wgpu.AdapterInfoGo{Device: "AMD", BackendType: wgpu.BackendTypeVulkan, AdapterType: wgpu.AdapterTypeIntegratedGPU}, false, true},
		{wgpu.AdapterInfoGo{Device: "Apple", BackendType: wgpu.BackendTypeMetal, AdapterType: wgpu.AdapterTypeIntegratedGPU}, false, true},
		{wgpu.AdapterInfoGo{Device: "GPU", BackendType: wgpu.BackendTypeD3D12, AdapterType: wgpu.AdapterTypeDiscreteGPU}, false, true},
		{wgpu.AdapterInfoGo{Device: "llvmpipe", BackendType: wgpu.BackendTypeVulkan, AdapterType: wgpu.AdapterTypeCPU}, false, false},
		{wgpu.AdapterInfoGo{Device: "llvmpipe", BackendType: wgpu.BackendTypeVulkan, AdapterType: wgpu.AdapterTypeCPU}, true, true},
		{wgpu.AdapterInfoGo{Device: "AMD", BackendType: wgpu.BackendTypeVulkan, AdapterType: wgpu.AdapterTypeIntegratedGPU}, true, false},
		{wgpu.AdapterInfoGo{Device: "CPU", BackendType: wgpu.BackendTypeOpenGL, AdapterType: wgpu.AdapterTypeCPU}, true, false},
		{wgpu.AdapterInfoGo{Device: "AMD RENOIR", BackendType: wgpu.BackendTypeOpenGL, AdapterType: wgpu.AdapterTypeUnknown}, false, false},
		{wgpu.AdapterInfoGo{Device: "SwiftShader", BackendType: wgpu.BackendTypeVulkan, AdapterType: wgpu.AdapterTypeIntegratedGPU}, false, false},
	} {
		identity, err := nativeAdapterIdentity(&entry.info, entry.ci)
		if entry.valid {
			if err != nil || (entry.ci && !strings.HasSuffix(identity, " mode=ci-software")) {
				t.Fatal(entry, identity, err)
			}
		} else if !errors.Is(err, ErrUnsupported) {
			t.Fatal(entry, identity, err)
		}

		if !entry.ci && entry.info.BackendType == wgpu.BackendTypeOpenGL && runtime.GOOS == "linux" {
			if err == nil || !strings.Contains(err.Error(), "Vulkan loader") || !strings.Contains(err.Error(), "driver (ICD)") {
				t.Fatal("unsupported Linux backend omitted the system Vulkan prerequisite", err)
			}
		}
	}
}
func TestNativeGoShaderSpecializationRetainsABIAndOnlyEnablesRequestedCounters(t *testing.T) {
	t.Parallel()

	data, err := nativeGPUShaderSource()
	if err != nil {
		t.Fatal(err)
	}

	source := string(data)

	ordinary, err := nativeSpecializeShader(source, false)
	if err != nil || ordinary != source {
		t.Fatal("ordinary shader changed", err)
	}

	traced, err := nativeSpecializeShader(source, true)
	if err != nil || strings.Replace(traced, "override lmProfileRays:bool=true;", "override lmProfileRays:bool=false;", 1) != source {
		t.Fatal("unexpected specialization", err)
	}

	for _, bad := range []string{strings.Replace(source, "lmPoolABI:u32=1u", "lmPoolABI:u32=2u", 1), strings.Replace(source, "lmSceneABI:u32=5u", "lmSceneABI:u32=4u", 1)} {
		if _, err := nativeSpecializeShader(bad, false); !errors.Is(err, ErrInvalid) {
			t.Fatal("changed shader ABI admitted", err)
		}
	}
}
