"use strict";

// One hardware device and pipeline set, with independently bounded sequential
// jobs. All lighting mathematics lives in the shared runtime WGSL module.
(async () => {
    let device;
    let stopped = false;
    const progress = {stage: "device admission"};
    const report = async error => {
        if (stopped) return;
        stopped = true;
        try { await fetch("error", {method: "POST", body: new TextEncoder().encode(progress.stage + "\n" + String(error && (error.stack || error))).subarray(0, 8192)}); } catch (_) {}
    };
    try {
        if (!navigator.gpu) throw new Error("WebGPU unavailable");
        const adapter = await navigator.gpu.requestAdapter({powerPreference: "high-performance", featureLevel: "compatibility"});
        if (!adapter) throw new Error("hardware WebGPU adapter unavailable");
        const info = adapter.info;
        let description = [info.vendor, info.architecture, info.device, info.description].filter(Boolean).join(" ");
        const gl = document.createElement("canvas").getContext("webgl2");
        const debug = gl && gl.getExtension("WEBGL_debug_renderer_info");
        const renderer = debug ? gl.getParameter(debug.UNMASKED_RENDERER_WEBGL) : "";
        if (info.isFallbackAdapter !== false || !renderer || /swiftshader|llvmpipe|softpipe|software/i.test(description + renderer)) {
            throw new Error("software WebGPU adapter rejected: " + description);
        }
        description = (description + " WebGPU OpenGLES; " + renderer).trim();
        device = await adapter.requestDevice();
        device.addEventListener("uncapturederror", event => { void report(new Error(event.error.message)); });
        device.lost.then(value => { if (value.reason !== "destroyed") void report(new Error("bake GPU lost (" + value.reason + "): " + value.message)); });
        const legacy = await loadPipelines(device, false);
        let production;
        const ready = await fetch("ready", {method: "POST", headers: {"X-Karty-Adapter": description}});
        if (!ready.ok) throw new Error("RNM session admission rejected");
        while (!stopped) {
            const next = await fetch("next");
            if (next.status === 204) continue;
            if (!next.ok) throw new Error("RNM session closed");
            const job = await next.json();
            if (!/^[1-9][0-9]{0,19}$/.test(job.id)) throw new Error("invalid RNM session job identity");
            progress.stage = "job " + job.id + " pipeline preparation";
            if (job.pathTraced && !production) production = await loadPipelines(device, true);
            const selected = job.pathTraced ? production : legacy;
            await bakeJob(device, selected.layout, selected.pipelines, job.id, description, Boolean(job.pathTraced), progress);
            progress.stage = "awaiting next job";
        }
    } catch (error) {
        await report(error);
    } finally {
        stopped = true;
        if (device) device.destroy();
    }
})();

async function loadPipelines(device, pathTraced) {
    device.pushErrorScope("validation");
    const response = await fetch(pathTraced ? "pathtrace.wgsl" : "shader.wgsl");
    if (!response.ok) throw new Error("shared RNM shader unavailable");
    const module = device.createShaderModule({label: "shared RNM session", code: await response.text()});
    const compilation = await module.getCompilationInfo();
    const errors = compilation.messages.filter(message => message.type === "error");
    if (errors.length) throw new Error(errors.map(message => `WGSL ${message.lineNum}:${message.linePos} ${message.message}`).join("\n"));
    const layout = device.createBindGroupLayout({entries: [
        {binding: 0, visibility: GPUShaderStage.COMPUTE, buffer: {type: "uniform", minBindingSize: 128}},
        {binding: 1, visibility: GPUShaderStage.COMPUTE, buffer: {type: "read-only-storage", minBindingSize: 4}},
        {binding: 2, visibility: GPUShaderStage.COMPUTE, buffer: {type: "read-only-storage", minBindingSize: 32}},
        {binding: 3, visibility: GPUShaderStage.COMPUTE, buffer: {type: "storage", minBindingSize: 16}},
        ...[4, 5, 6].map(binding => ({binding, visibility: GPUShaderStage.COMPUTE, storageTexture: {access: "write-only", format: "rgba8unorm", viewDimension: "2d"}})),
        ...(pathTraced ? [{binding: 7, visibility: GPUShaderStage.COMPUTE, buffer: {type: "read-only-storage", minBindingSize: 16}}] : [])
    ]});
    const pipelineLayout = device.createPipelineLayout({bindGroupLayouts: [layout]});
    const pipelines = {};
    for (const entryPoint of (pathTraced ? ["pathtrace_rnm", "resolve_pathtrace_rnm"] : ["bake_rnm", "resolve_rnm", "bake_rnm_patches"])) {
        pipelines[entryPoint] = await device.createComputePipelineAsync({layout: pipelineLayout, compute: {module, entryPoint}});
    }
    const validation = await device.popErrorScope();
    if (validation) throw validation;
    return {layout, pipelines};
}

async function bakeJob(device, layout, pipelines, id, description, pathTraced, progress) {
    const buffers = [], images = [];
    let readback;
    try {
        device.pushErrorScope("validation");
        progress.stage = "job " + id + " input upload";
        const prefix = "job/" + id + "/";
        const files = await Promise.all(["parameters.bin", "scene.bin", "points.bin", ...(pathTraced ? ["geometric.bin"] : [])].map(async name => {
            const response = await fetch(prefix + name);
            if (!response.ok) throw new Error("RNM job input unavailable: " + name);
            return response.arrayBuffer();
        }));
        if (files[0].byteLength !== 128 || files[1].byteLength > 134217728 || files[2].byteLength > 131072) throw new Error("RNM job byte bounds");
        const parameters = new DataView(files[0]);
        const width = parameters.getUint32(0, true), height = parameters.getUint32(4, true);
        const rows = parameters.getUint32(60, true), patchCount = parameters.getUint32(116, true);
        const scratchSize = width * height * 48 + patchCount * 16;
        if (!width || !height || width > 2048 || height > 2048 || !rows || rows > height || scratchSize > 134217728) throw new Error("RNM job dimensions");
        if (pathTraced && files[3].byteLength !== width * height * 16) throw new Error("RNM geometric normal bounds");
        const entries = files.map((bytes, index) => {
            const binding = index === 3 ? 7 : index;
            const buffer = device.createBuffer({label: "RNM job input " + binding, size: bytes.byteLength, usage: GPUBufferUsage.COPY_DST | (binding === 0 ? GPUBufferUsage.UNIFORM : GPUBufferUsage.STORAGE)});
            buffers.push(buffer);
            device.queue.writeBuffer(buffer, 0, bytes);
            return {binding, resource: {buffer}};
        });
        const scratch = device.createBuffer({label: "RNM job coefficients", size: scratchSize, usage: GPUBufferUsage.STORAGE});
        buffers.push(scratch);
        entries.push({binding: 3, resource: {buffer: scratch}});
        for (let index = 0; index < 3; index++) {
            const image = device.createTexture({label: "RNM job output " + index, size: [width, height], format: "rgba8unorm", usage: GPUTextureUsage.STORAGE_BINDING | GPUTextureUsage.COPY_SRC});
            images.push(image);
            entries.push({binding: index + 4, resource: image.createView()});
        }
        const group = device.createBindGroup({layout, entries});
        const dispatch = (entryPoint, x, y = 1) => {
            const encoder = device.createCommandEncoder();
            const pass = encoder.beginComputePass();
            pass.setPipeline(pipelines[entryPoint]);
            pass.setBindGroup(0, group);
            pass.dispatchWorkgroups(x, y);
            pass.end();
            device.queue.submit([encoder.finish()]);
        };
        if (!pathTraced && patchCount > 0 && parameters.getUint32(48, true) > 0) dispatch("bake_rnm_patches", Math.ceil(patchCount / 64));
        for (let first = 0; first < height; first += rows) {
            const count = Math.min(rows, height - first);
            parameters.setUint32(56, first, true);
            parameters.setUint32(60, count, true);
            if (pathTraced) {
                // This is offline GPU scheduling, independent of sample quality
                // and runtime loading. Keep long BVH loops out of one giant
                // non-preemptible GL dispatch and do not queue an entire atlas.
                const samples = parameters.getUint32(44, true);
                const samplesPerDispatch = 16;
                for (let sample = 0; sample < samples; sample += samplesPerDispatch) {
                    const sampleCount = Math.min(samplesPerDispatch, samples - sample);
                    parameters.setUint32(120, sample, true);
                    parameters.setUint32(124, sampleCount, true);
                    progress.stage = `job ${id} rows ${first}..${first + count}/${height} samples ${sample}..${sample + sampleCount}/${samples}`;
                    device.queue.writeBuffer(buffers[0], 0, files[0]);
                    dispatch("pathtrace_rnm", Math.ceil(width / 8), Math.ceil(count / 8));
                    await device.queue.onSubmittedWorkDone();
                }
            } else {
                device.queue.writeBuffer(buffers[0], 0, files[0]);
                dispatch("bake_rnm", Math.ceil(width / 8), Math.ceil(count / 8));
            }
        }
        progress.stage = "job " + id + " output resolve/readback";
        dispatch(pathTraced ? "resolve_pathtrace_rnm" : "resolve_rnm", Math.ceil(width / 8), Math.ceil(height / 8));
        const row = Math.ceil(width * 4 / 256) * 256;
        readback = device.createBuffer({label: "RNM job readback", size: row * height * 3, usage: GPUBufferUsage.MAP_READ | GPUBufferUsage.COPY_DST});
        buffers.push(readback);
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
        const response = await fetch(prefix + "result", {method: "POST", headers: {"X-Karty-Adapter": description}, body: result});
        if (!response.ok) throw new Error("RNM job result rejected");
    } finally {
        if (readback && readback.mapState === "mapped") readback.unmap();
        for (const image of images) image.destroy();
        for (const buffer of buffers) buffer.destroy();
    }
}
