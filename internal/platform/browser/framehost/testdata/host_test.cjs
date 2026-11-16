const assert = require("node:assert/strict");
const originalFailure = Object.freeze({ testFailure: true });

function gpuFixture() {
  const gpu = { mode: "", calls: [], frames: 0, submits: 0, lastArray: null };
  class Pass {
    #kind;
    #ended = false;
    constructor(kind) { this.#kind = kind; }
    record(method, args) {
      assert.equal(this.#ended, false);
      if (gpu.mode === "commandThrow") throw originalFailure;
      gpu.calls.push([this.#kind, method, args]);
    }
    setPipeline(pipeline) { this.record("pipeline", [pipeline]); }
    setVertexBuffer(slot, buffer) { this.record("vertices", [slot, buffer]); }
    setIndexBuffer(buffer, format) { this.record("indices", [buffer, format]); }
    setBindGroup(index, group) { this.record("group", [index, group]); }
    setScissorRect(x, y, width, height) { this.record("scissor", [x, y, width, height]); }
    drawIndexed(indices, instances) { this.record("indexed", [indices, instances]); }
    draw(vertices, instances, firstVertex, firstInstance) { this.record("draw", [vertices, instances, firstVertex, firstInstance]); }
    dispatchWorkgroups(x, y, z) { this.record("dispatch", [x, y, z]); }
    end() { this.record("end", []); this.#ended = true; }
  }
  class Encoder {
    #frame;
    constructor(frame) { this.#frame = frame; }
    beginRenderPass(descriptor) {
      assert.equal(this.#frame, gpu.frames);
      gpu.inspect?.(descriptor);
      if (gpu.mode === "renderThrow") throw originalFailure;
      return gpu.mode === "renderMissing" ? null : new Pass("render");
    }
    beginComputePass() {
      assert.equal(this.#frame, gpu.frames);
      if (gpu.mode === "computeThrow") throw originalFailure;
      return gpu.mode === "computeMissing" ? null : new Pass("compute");
    }
    finish() {
      assert.equal(this.#frame, gpu.frames);
      if (gpu.mode === "finishThrow") throw originalFailure;
      return gpu.mode === "finishMissing" ? null : { frame: this.#frame };
    }
  }
  class Context {
    #branded = true;
    getCurrentTexture() {
      assert.equal(this.#branded, true);
      if (gpu.mode === "textureThrow") throw originalFailure;
      if (gpu.mode === "textureMissing") return null;
      const frame = ++gpu.frames;
      return {
        createView() {
          if (gpu.mode === "viewThrow") throw originalFailure;
          gpu.view = { frame };
          return gpu.mode === "viewMissing" ? null : gpu.view;
        }
      };
    }
  }
  class Device {
    #branded = true;
    createCommandEncoder() {
      assert.equal(this.#branded, true);
      if (gpu.mode === "encoderThrow") throw originalFailure;
      return gpu.mode === "encoderMissing" ? null : new Encoder(gpu.frames);
    }
  }
  class Queue {
    #branded = true;
    submit(commands) {
      assert.equal(this.#branded, true);
      assert.equal(commands.length, 1);
      assert.equal(commands[0].frame, gpu.frames);
      if (gpu.lastArray) assert.equal(commands, gpu.lastArray);
      gpu.lastArray = commands;
      if (gpu.mode === "submitThrow") throw originalFailure;
      gpu.submits++;
    }
  }
  gpu.context = new Context();
  gpu.device = new Device();
  gpu.queue = new Queue();
  return gpu;
}

function assertDiscarded(host, gpu) {
  assert.equal(host.submit(gpu.queue), false);
  assert.throws(() => host.encoder.beginComputePass(), /frame missing/);
  if (gpu.lastArray) assert.equal(gpu.lastArray[0], undefined);
}

function testCommandsAndCycles() {
  const host = __ashuraCreateFrameHost(), gpu = gpuFixture();
  const facades = [host.encoder, host.renderPass, host.computePass, host.surface];
  assert.equal(host.version, 1);
  assert.deepEqual(Object.keys(host).sort(), ["begin", "close", "computePass", "encoder", "renderPass", "reset", "submit", "surface", "version"]);
  const pipeline = {}, buffer = {}, group = {}, retainedView = {};
  const descriptor = { colorAttachments: [
    { view: host.surface }, null, { view: retainedView, resolveTarget: host.surface }
  ] };
  gpu.inspect = d => {
    assert.equal(d, descriptor);
    assert.equal(d.colorAttachments[0].view, gpu.view);
    assert.equal(d.colorAttachments[1], null);
    assert.equal(d.colorAttachments[2].view, retainedView);
    assert.equal(d.colorAttachments[2].resolveTarget, gpu.view);
  };
  for (let index = 0; index < 1000; index++) {
    gpu.calls.length = 0;
    assert.equal(host.begin(gpu.context, gpu.device), true);
    assert.equal(host.encoder.beginRenderPass(descriptor), true);
    assert.equal(descriptor.colorAttachments[0].view, host.surface);
    assert.equal(descriptor.colorAttachments[2].resolveTarget, host.surface);
    host.renderPass.setPipeline(pipeline);
    host.renderPass.setVertexBuffer(0, buffer);
    host.renderPass.setIndexBuffer(buffer, "uint32");
    host.renderPass.setBindGroup(1, group);
    host.renderPass.setScissorRect(1, 2, 3, 4);
    host.renderPass.drawIndexed(12, 5);
    host.renderPass.draw(6, 3, 0, 7);
    host.renderPass.end();
    assert.equal(host.encoder.beginComputePass(), true);
    host.computePass.setPipeline(pipeline);
    host.computePass.setBindGroup(0, group);
    host.computePass.dispatchWorkgroups(1, 2, 3);
    host.computePass.end();
    assert.deepEqual(gpu.calls, [
      ["render", "pipeline", [pipeline]], ["render", "vertices", [0, buffer]],
      ["render", "indices", [buffer, "uint32"]], ["render", "group", [1, group]],
      ["render", "scissor", [1, 2, 3, 4]], ["render", "indexed", [12, 5]],
      ["render", "draw", [6, 3, 0, 7]], ["render", "end", []],
      ["compute", "pipeline", [pipeline]], ["compute", "group", [0, group]],
      ["compute", "dispatch", [1, 2, 3]], ["compute", "end", []]
    ]);
    assert.equal(host.submit(gpu.queue), true);
    assert.equal(gpu.lastArray[0], undefined);
    assert.deepEqual([host.encoder, host.renderPass, host.computePass, host.surface], facades);
    assert.equal(host.submit(gpu.queue), false);
  }
  assert.equal(gpu.submits, 1000);
  host.reset(); host.reset(); host.close(); host.close(); host.reset();
  assert.equal(host.begin(gpu.context, gpu.device), false);
  assertDiscarded(host, gpu);
}

function testFailures() {
  for (const mode of ["textureMissing", "viewMissing", "encoderMissing"]) {
    const host = __ashuraCreateFrameHost(), gpu = gpuFixture();
    gpu.mode = mode;
    assert.equal(host.begin(gpu.context, gpu.device), false);
    assertDiscarded(host, gpu);
    gpu.mode = "";
    assert.equal(host.begin(gpu.context, gpu.device), true);
    assert.equal(host.submit(gpu.queue), true);
  }
  for (const mode of ["textureThrow", "viewThrow", "encoderThrow"]) {
    const host = __ashuraCreateFrameHost(), gpu = gpuFixture();
    gpu.mode = mode;
    assert.throws(() => host.begin(gpu.context, gpu.device), error => error === originalFailure);
    assertDiscarded(host, gpu);
  }
  for (const mode of ["renderMissing", "renderThrow", "computeMissing", "computeThrow"]) {
    const host = __ashuraCreateFrameHost(), gpu = gpuFixture();
    assert.equal(host.begin(gpu.context, gpu.device), true);
    gpu.mode = mode;
    const descriptor = { colorAttachments: [{ view: host.surface, resolveTarget: host.surface }] };
    const begin = mode.startsWith("render") ? () => host.encoder.beginRenderPass(descriptor) : () => host.encoder.beginComputePass();
    assert.throws(begin, error => mode.endsWith("Throw") ? error === originalFailure : /no pass/.test(error.message));
    assert.equal(descriptor.colorAttachments[0].view, host.surface);
    assert.equal(descriptor.colorAttachments[0].resolveTarget, host.surface);
    assertDiscarded(host, gpu);
  }
  for (const mode of ["finishMissing", "finishThrow", "submitThrow"]) {
    const host = __ashuraCreateFrameHost(), gpu = gpuFixture();
    assert.equal(host.begin(gpu.context, gpu.device), true);
    gpu.mode = mode;
    if (mode === "finishMissing") assert.equal(host.submit(gpu.queue), false);
    else assert.throws(() => host.submit(gpu.queue), error => error === originalFailure);
    assertDiscarded(host, gpu);
    gpu.mode = "";
    assert.equal(host.begin(gpu.context, gpu.device), true);
    assert.equal(host.submit(gpu.queue), true);
  }
}

function testInvalidState() {
  const host = __ashuraCreateFrameHost(), gpu = gpuFixture();
  assert.throws(() => host.renderPass.drawIndexed(3, 1), /no active render/);
  assert.equal(host.begin(gpu.context, gpu.device), true);
  assert.throws(() => host.encoder.beginRenderPass({ colorAttachments: Array(9) }), /attachment/);
  assertDiscarded(host, gpu);
  assert.equal(host.begin(gpu.context, gpu.device), true);
  assert.equal(host.encoder.beginRenderPass({ colorAttachments: [] }), true);
  assert.throws(() => host.encoder.beginComputePass(), /already active/);
  assertDiscarded(host, gpu);
  assert.equal(host.begin(gpu.context, gpu.device), true);
  assert.equal(host.encoder.beginComputePass(), true);
  assert.throws(() => host.renderPass.end(), /no active render/);
  assertDiscarded(host, gpu);
  assert.equal(host.begin(gpu.context, gpu.device), true);
  assert.equal(host.encoder.beginComputePass(), true);
  assert.throws(() => host.submit(gpu.queue), /active pass/);
  assertDiscarded(host, gpu);
  assert.equal(host.begin(gpu.context, gpu.device), true);
  assert.equal(host.encoder.beginRenderPass({ colorAttachments: [] }), true);
  gpu.mode = "commandThrow";
  assert.throws(() => host.renderPass.setPipeline({}), error => error === originalFailure);
  assertDiscarded(host, gpu);
}

function testMeshCommands() {
  const host = __ashuraCreateFrameHost(), gpu = gpuFixture();
  const vertices = {}, instances = {}, uv = {}, indices = {}, group = {};
  for (const lightmap of [uv, undefined]) {
    assert.equal(host.begin(gpu.context, gpu.device), true);
    host.encoder.beginRenderPass({ colorAttachments: [] });
    gpu.calls.length = 0;
    host.renderPass.drawMesh(vertices, instances, lightmap, indices, "uint32", group, 24, 5000);
    const expected = [
      ["render", "vertices", [0, vertices]], ["render", "vertices", [1, instances]]
    ];
    if (lightmap !== undefined) expected.push(["render", "vertices", [2, uv]]);
    expected.push(["render", "indices", [indices, "uint32"]],
      ["render", "group", [0, group]], ["render", "indexed", [24, 5000]]);
    assert.deepEqual(gpu.calls, expected);
    host.renderPass.end();
    assert.equal(host.submit(gpu.queue), true);
  }
  host.begin(gpu.context, gpu.device);
  host.encoder.beginRenderPass({ colorAttachments: [] });
  gpu.mode = "commandThrow";
  assert.throws(() => host.renderPass.drawMesh(vertices, instances, uv, indices, "uint32", group, 24, 5000),
    error => error === originalFailure);
  assertDiscarded(host, gpu);
}

testCommandsAndCycles();
testMeshCommands();
testFailures();
testInvalidState();
