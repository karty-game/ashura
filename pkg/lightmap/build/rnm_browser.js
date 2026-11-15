"use strict";

(async () => {
    let device;
    try {
        if (!navigator.gpu) throw new Error("WebGPU unavailable");
        const adapter = await navigator.gpu.requestAdapter({powerPreference: "high-performance", featureLevel: "compatibility"});
        if (!adapter) throw new Error("hardware WebGPU adapter unavailable");
        const info = adapter.info;
        let description = [info.vendor, info.architecture, info.device, info.description].filter(Boolean).join(" ");
        // Chromium compatibility adapters may hide identifying strings. This
        // executor explicitly selects the OpenGLES WebGPU backend, so record
        // the accelerated ANGLE renderer as well and reject software there.
        const gl = document.createElement("canvas").getContext("webgl2");
        const debug = gl && gl.getExtension("WEBGL_debug_renderer_info");
        const renderer = debug ? gl.getParameter(debug.UNMASKED_RENDERER_WEBGL) : "";
        if (info.isFallbackAdapter !== false || !renderer || /swiftshader|llvmpipe|softpipe|software/i.test(description + renderer)) {
            throw new Error("software WebGPU adapter rejected: " + description);
        }
        description = (description + " WebGPU OpenGLES; " + renderer).trim();
        device = await adapter.requestDevice();
        device.lost.then(value => { if (value.reason !== "destroyed") throw new Error("bake GPU lost: " + value.message); });
        device.pushErrorScope("validation");
        const files = await Promise.all(["shader.wgsl", "parameters.bin", "scene.bin", "points.bin"].map(async name => {
            const response = await fetch(name);
            if (!response.ok) throw new Error("bake input unavailable: " + name);
            return response.arrayBuffer();
        }));
        const parameters = new DataView(files[1]);
        const width = parameters.getUint32(0, true), height = parameters.getUint32(4, true);
        const rows = parameters.getUint32(60, true);
        const entries = files.slice(1).map((bytes, binding) => {
            const buffer = device.createBuffer({label: "RNM input " + binding, size: bytes.byteLength, usage: GPUBufferUsage.COPY_DST | (binding === 0 ? GPUBufferUsage.UNIFORM : GPUBufferUsage.STORAGE)});
            device.queue.writeBuffer(buffer, 0, bytes);
            return {binding, resource: {buffer}};
        });
        const patchCount = parameters.getUint32(116, true);
        const scratchSize = width * height * 48 + patchCount * 16;
        const scratch = device.createBuffer({label: "RNM coefficients", size: scratchSize, usage: GPUBufferUsage.STORAGE});
        entries.push({binding: 3, resource: {buffer: scratch}});
        const images = Array.from({length: 3}, (_, index) => {
            const image = device.createTexture({label: "RNM output " + index, size: [width, height], format: "rgba8unorm", usage: GPUTextureUsage.STORAGE_BINDING | GPUTextureUsage.COPY_SRC});
            entries.push({binding: index + 4, resource: image.createView()});
            return image;
        });
        const module = device.createShaderModule({label: "shared RNM bake", code: new TextDecoder().decode(files[0])});
        const compilation = await module.getCompilationInfo();
        const compilationErrors = compilation.messages.filter(message => message.type === "error");
        if (compilationErrors.length) throw new Error(compilationErrors.map(message => `WGSL ${message.lineNum}:${message.linePos} ${message.message}`).join("\n"));
        const layout = device.createBindGroupLayout({entries: [
            {binding: 0, visibility: GPUShaderStage.COMPUTE, buffer: {type: "uniform", minBindingSize: 128}},
            {binding: 1, visibility: GPUShaderStage.COMPUTE, buffer: {type: "read-only-storage", minBindingSize: files[2].byteLength}},
            {binding: 2, visibility: GPUShaderStage.COMPUTE, buffer: {type: "read-only-storage", minBindingSize: files[3].byteLength}},
            {binding: 3, visibility: GPUShaderStage.COMPUTE, buffer: {type: "storage", minBindingSize: scratchSize}},
            ...[4, 5, 6].map(binding => ({binding, visibility: GPUShaderStage.COMPUTE, storageTexture: {access: "write-only", format: "rgba8unorm", viewDimension: "2d"}}))
        ]});
        const pipelineLayout = device.createPipelineLayout({bindGroupLayouts: [layout]});
        const bake = await device.createComputePipelineAsync({layout: pipelineLayout, compute: {module, entryPoint: "bake_rnm"}});
        const resolve = await device.createComputePipelineAsync({layout: pipelineLayout, compute: {module, entryPoint: "resolve_rnm"}});
        const group = device.createBindGroup({layout, entries});
        const dispatch = (pipeline, y) => {
            const encoder = device.createCommandEncoder();
            const pass = encoder.beginComputePass();
            pass.setPipeline(pipeline);
            pass.setBindGroup(0, group);
            pass.dispatchWorkgroups(Math.ceil(width / 8), Math.ceil(y / 8));
            pass.end();
            device.queue.submit([encoder.finish()]);
        };
        if (patchCount > 0 && parameters.getUint32(48, true) > 0) {
            const patches = await device.createComputePipelineAsync({layout: pipelineLayout, compute: {module, entryPoint: "bake_rnm_patches"}});
            const encoder = device.createCommandEncoder();
            const pass = encoder.beginComputePass();
            pass.setPipeline(patches);
            pass.setBindGroup(0, group);
            pass.dispatchWorkgroups(Math.ceil(patchCount / 64));
            pass.end();
            device.queue.submit([encoder.finish()]);
        }
        for (let first = 0; first < height; first += rows) {
            const count = Math.min(rows, height - first);
            parameters.setUint32(56, first, true);
            parameters.setUint32(60, count, true);
            device.queue.writeBuffer(entries[0].resource.buffer, 0, files[1]);
            // Submit each interval before changing its uniform snapshot, exactly
            // as the runtime's one Record per frame scheduling does.
            dispatch(bake, count);
        }
        dispatch(resolve, height);
        const row = Math.ceil(width * 4 / 256) * 256;
        const readback = device.createBuffer({label: "RNM readback", size: row * height * 3, usage: GPUBufferUsage.MAP_READ | GPUBufferUsage.COPY_DST});
        const encoder = device.createCommandEncoder();
        images.forEach((image, index) => encoder.copyTextureToBuffer({texture: image}, {buffer: readback, offset: index * row * height, bytesPerRow: row, rowsPerImage: height}, [width, height]));
        device.queue.submit([encoder.finish()]);
        await readback.mapAsync(GPUMapMode.READ);
        const validation = await device.popErrorScope();
        if (validation) throw validation;
        const mapped = new Uint8Array(readback.getMappedRange());
        const result = new Uint8Array(width * height * 12);
        for (let index = 0; index < 3; index++) {
            for (let y = 0; y < height; y++) result.set(mapped.subarray(index * row * height + y * row, index * row * height + y * row + width * 4), index * width * height * 4 + y * width * 4);
        }
        readback.unmap();
        const response = await fetch("result", {method: "POST", headers: {"X-Karty-Adapter": description}, body: result});
        if (!response.ok) throw new Error("offline bake result rejected");
    } catch (error) {
        await fetch("error", {method: "POST", body: String(error && (error.stack || error))});
    } finally {
        if (device) device.destroy();
    }
})();
