// Engine-owned frame references stay in JavaScript, outside syscall/js finalizers.
(() => {
  "use strict";
  const maxAttachments = 8;

  globalThis.__ashuraCreateFrameHost = function createFrameHost() {
    const surface = Object.freeze({});
    const submitted = [undefined];
    let texture, view, encoder, pass, passKind;
    let closed = false;

    function reset() {
      texture = view = encoder = pass = passKind = undefined;
      submitted[0] = undefined;
    }

    function fail(error) {
      reset();
      throw error;
    }

    function active(kind) {
      if (closed || !encoder || !pass || passKind !== kind) {
        fail(new Error("Ashura frame host: no active " + kind + " pass"));
      }
      return pass;
    }

    // Fixed arities avoid another argument array on each forwarded GPU command.
    function call1(kind, method, a) {
      try { active(kind)[method](a); } catch (error) { fail(error); }
    }
    function call2(kind, method, a, b) {
      try { active(kind)[method](a, b); } catch (error) { fail(error); }
    }
    function call3(kind, method, a, b, c) {
      try { active(kind)[method](a, b, c); } catch (error) { fail(error); }
    }
    function call4(kind, method, a, b, c, d) {
      try { active(kind)[method](a, b, c, d); } catch (error) { fail(error); }
    }
    function end(kind) {
      try {
        active(kind).end();
        pass = passKind = undefined;
      } catch (error) { fail(error); }
    }

    function beginPass(kind, descriptor) {
      let attachments, attachmentCount = 0, viewMask = 0, resolveMask = 0;
      try {
        if (closed || !encoder || pass) {
          throw new Error("Ashura frame host: frame missing or pass already active");
        }
        if (kind === "render") {
          attachments = descriptor.colorAttachments;
          if (!Array.isArray(attachments) || attachments.length > maxAttachments) {
            throw new Error("Ashura frame host: invalid color attachment array");
          }
          attachmentCount = attachments.length;
          for (let index = 0; index < attachmentCount; index++) {
            const attachment = attachments[index];
            if (!attachment) continue;
            if (attachment.view === surface) {
              viewMask |= 1 << index;
              attachment.view = view;
            }
            if (attachment.resolveTarget === surface) {
              resolveMask |= 1 << index;
              attachment.resolveTarget = view;
            }
          }
          pass = encoder.beginRenderPass(descriptor);
        } else {
          pass = encoder.beginComputePass();
        }
        if (!pass) throw new Error("Ashura frame host: GPU returned no pass encoder");
        passKind = kind;
        return true;
      } catch (error) {
        fail(error);
      } finally {
        // WebGPU consumes its descriptor synchronously. Keep cached descriptors
        // free of transient surface references, including when GPU creation throws.
        for (let index = 0; index < attachmentCount; index++) {
          if (viewMask & (1 << index)) attachments[index].view = surface;
          if (resolveMask & (1 << index)) attachments[index].resolveTarget = surface;
        }
      }
    }

    const renderPass = Object.freeze({
      setPipeline(pipeline) { call1("render", "setPipeline", pipeline); },
      setVertexBuffer(slot, buffer) { call2("render", "setVertexBuffer", slot, buffer); },
      setVertexBufferRange(slot, buffer, offset, size) { call4("render", "setVertexBuffer", slot, buffer, offset, size); },
      setIndexBuffer(buffer, format) { call2("render", "setIndexBuffer", buffer, format); },
      setBindGroup(index, group) { call2("render", "setBindGroup", index, group); },
      setScissorRect(x, y, width, height) { call4("render", "setScissorRect", x, y, width, height); },
      drawIndexed(indices, instances) { call2("render", "drawIndexed", indices, instances); },
      drawMesh(vertices, instances, uv, indices, format, group, indexCount, instanceCount) {
        try {
          const target = active("render");
          target.setVertexBuffer(0, vertices);
          target.setVertexBuffer(1, instances);
          if (uv !== undefined) target.setVertexBuffer(2, uv);
          target.setIndexBuffer(indices, format);
          target.setBindGroup(0, group);
          target.drawIndexed(indexCount, instanceCount);
        } catch (error) { fail(error); }
      },
      draw(vertices, instances, firstVertex, firstInstance) { call4("render", "draw", vertices, instances, firstVertex, firstInstance); },
      end() { end("render"); }
    });
    const computePass = Object.freeze({
      setPipeline(pipeline) { call1("compute", "setPipeline", pipeline); },
      setBindGroup(index, group) { call2("compute", "setBindGroup", index, group); },
      dispatchWorkgroups(x, y, z) { call3("compute", "dispatchWorkgroups", x, y, z); },
      end() { end("compute"); }
    });
    const encoderFacade = Object.freeze({
      beginRenderPass(descriptor) { return beginPass("render", descriptor); },
      beginComputePass() { return beginPass("compute"); },
      copyParticleState(source, destination, size) {
        try {
          if (closed || !encoder || pass) throw new Error("Ashura frame host: particle copy requires an idle frame encoder");
          encoder.copyBufferToBuffer(source, 0, destination, 0, size);
        } catch (error) { fail(error); }
      }
    });

    return Object.freeze({
      version: 1, encoder: encoderFacade, renderPass, computePass, surface,
      begin(context, device) {
        reset();
        if (closed) return false;
        try {
          texture = context.getCurrentTexture();
          if (!texture) { reset(); return false; }
          view = texture.createView();
          if (!view) { reset(); return false; }
          encoder = device.createCommandEncoder();
          if (!encoder) { reset(); return false; }
          return true;
        } catch (error) { fail(error); }
      },
      submit(queue) {
        try {
          if (closed || !encoder) return false;
          if (pass) throw new Error("Ashura frame host: cannot submit an active pass");
          submitted[0] = encoder.finish();
          if (!submitted[0]) return false;
          queue.submit(submitted);
          return true;
        } finally { reset(); }
      },
      reset,
      close() { closed = true; reset(); }
    });
  };
})();
