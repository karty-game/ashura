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
        const files = await Promise.all(["shader.wgsl", "parameters.bin", "nodes.bin", "triangles.bin", "receivers.bin", "padding.bin"].map(async name => {
            const response = await fetch(name);
            if (!response.ok) throw new Error("bake input unavailable: " + name);
            return response.arrayBuffer();
        }));
        const parameters = new DataView(files[1]);
        const width = parameters.getUint32(32, true), height = parameters.getUint32(36, true);
        const entries = files.slice(1).map((bytes, binding) => {
            const buffer = device.createBuffer({label: "lightmap input " + binding, size: bytes.byteLength, usage: GPUBufferUsage.COPY_DST | (binding === 0 ? GPUBufferUsage.UNIFORM : GPUBufferUsage.STORAGE)});
            device.queue.writeBuffer(buffer, 0, bytes);
            return {binding, resource: {buffer}};
        });
        const image = device.createTexture({label: "lightmap output", size: [width, height], format: "rgba8unorm", usage: GPUTextureUsage.STORAGE_BINDING | GPUTextureUsage.COPY_SRC});
        entries.push({binding: 5, resource: image.createView()});
        const module = device.createShaderModule({label: "shared direct bake", code: new TextDecoder().decode(files[0])});
        const pipeline = await device.createComputePipelineAsync({layout: "auto", compute: {module, entryPoint: "bake_direct"}});
        const group = device.createBindGroup({layout: pipeline.getBindGroupLayout(0), entries});
        const row = Math.ceil(width * 4 / 256) * 256;
        const readback = device.createBuffer({label: "lightmap readback", size: row * height, usage: GPUBufferUsage.MAP_READ | GPUBufferUsage.COPY_DST});
        const encoder = device.createCommandEncoder();
        const pass = encoder.beginComputePass();
        pass.setPipeline(pipeline);
        pass.setBindGroup(0, group);
        pass.dispatchWorkgroups(Math.ceil(width / 8), Math.ceil(height / 8));
        pass.end();
        encoder.copyTextureToBuffer({texture: image}, {buffer: readback, bytesPerRow: row, rowsPerImage: height}, [width, height]);
        device.queue.submit([encoder.finish()]);
        await readback.mapAsync(GPUMapMode.READ);
        const validation = await device.popErrorScope();
        if (validation) throw validation;
        const mapped = new Uint8Array(readback.getMappedRange());
        const result = new Uint8Array(width * height * 4);
        for (let y = 0; y < height; y++) result.set(mapped.subarray(y * row, y * row + width * 4), y * width * 4);
        readback.unmap();
        const response = await fetch("result", {method: "POST", headers: {"X-Karty-Adapter": description}, body: result});
        if (!response.ok) throw new Error("offline bake result rejected");
    } catch (error) {
        await fetch("error", {method: "POST", body: String(error && (error.stack || error))});
    } finally {
        if (device) device.destroy();
    }
})();
