// Shared by Ashura's native and browser particle renderers. Built-in sampling
// and seeded randomness follow the MIT particlr runtime.
struct Particle {
    position: vec3f,
    noisePhase: f32,
    velocity: vec3f,
    spin: f32,
    age: f32,
    life: f32,
    size: f32,
    rotation: f32,
}

struct ParticleState {
    rng: u32,
    particles: array<Particle>,
}

struct Layer {
    lifeSpeed: vec4f,
    sizeRotation: vec4f,
    spinGravity: vec4f,
    shape: vec4f,
    noise: vec4f,
    uv: vec4f,
    seedSurfaceAspect: vec4u,
    samples: array<vec4f, 512>,
    // Spatial shape, spawn radius, cosine of cone half-angle, upward offset.
    spatial: vec4f,
    // Render mode, speed scale, minimum stretch, maximum stretch.
    renderStyle: vec4f,
    // World X and Z offsets, initial rise speed, downward emitter acceleration.
    placement: vec4f,
}

struct Frame {
    dt: f32,
    clock: f32,
    // Screen origin XY and viewport WH, or world origin XYZ and world scale.
    origin: vec2f,
    extent: vec2f,
    births: u32,
    reset: u32,
}

@group(0) @binding(0) var<storage, read_write> state: ParticleState;
@group(0) @binding(1) var<uniform> layer: Layer;
@group(0) @binding(2) var<uniform> frame: Frame;

@group(1) @binding(0) var page: texture_2d<f32>;
@group(1) @binding(1) var pageSampler: sampler;

const TAU = 6.283185307179586;
const RAD = 0.017453292519943295;
const RNG_STEP = 0x6d2b79f5u;
const SPHERE = 1.0;
const CONE = 2.0;
const GROUND = 3.0;

fn randomWord(word: u32) -> f32 {
    var value = (word ^ (word >> 15u)) * (1u | word);
    value = (value + (value ^ (value >> 7u)) * (61u | value)) ^ value;
    return f32((value ^ (value >> 14u)) >> 8u) / 16777216.0;
}

fn nextRandom(rng: ptr<function, u32>) -> f32 {
    *rng += RNG_STEP;
    return randomWord(*rng);
}

fn spawnPosition(random: vec3f) -> vec3f {
    let shape = layer.shape;
    let surface = layer.seedSurfaceAspect.y != 0u;
    switch u32(shape.x) {
        case 1u, 2u: {
            var angle = random.x * shape.w;
            var innerRadius = shape.z;
            if shape.x == 2.0 {
                angle += shape.z - shape.w * 0.5;
                innerRadius = 0.0;
            }
            let radius = select(sqrt(mix(innerRadius * innerRadius, shape.y * shape.y, random.y)), shape.y, surface);
            let direction = select(angle, random.z * TAU, radius == 0.0 && shape.x == 1.0);
            return vec3f(radius * vec2f(cos(angle), sin(angle)), direction);
        }
        case 3u: {
            let size = shape.yz;
            var point = (random.xy - vec2f(0.5)) * size;
            if surface {
                let half = size * 0.5;
                let distance = random.x * 2.0 * (size.x + size.y);
                if distance < size.x {
                    point = vec2f(distance - half.x, -half.y);
                } else if distance < size.x + size.y {
                    point = vec2f(half.x, distance - size.x - half.y);
                } else if distance < 2.0 * size.x + size.y {
                    point = vec2f(half.x - (distance - size.x - size.y), half.y);
                } else {
                    point = vec2f(-half.x, half.y - (distance - 2.0 * size.x - size.y));
                }
            }
            return vec3f(point, -TAU * 0.25);
        }
        case 4u: {
            return vec3f((random.x - 0.5) * shape.y, 0.0, -TAU * 0.25);
        }
        default: {
            return vec3f(0.0, 0.0, random.z * TAU);
        }
    }
}

struct InitialMotion {
    position: vec3f,
    velocity: vec3f,
}

fn spawnMotion(random: vec3f, speed: f32, rng: ptr<function, u32>) -> InitialMotion {
    let azimuth = random.x * TAU;
    if layer.spatial.x == SPHERE {
        let height = random.y * 2.0 - 1.0;
        let ring = sqrt(max(0.0, 1.0 - height * height));
        let direction = vec3f(ring * cos(azimuth), height, ring * sin(azimuth));
        let radius = layer.spatial.y * pow(random.z, 1.0 / 3.0);
        return InitialMotion(direction * radius, direction * speed);
    }
    if layer.spatial.x == CONE {
        let angle = layer.shape.z;
        let axis = vec3f(cos(angle), sin(angle), 0.0);
        let across = vec3f(-axis.y, axis.x, 0.0);
        let depth = vec3f(0.0, 0.0, 1.0);
        let cosine = mix(layer.spatial.z, 1.0, random.y);
        let sine = sqrt(max(0.0, 1.0 - cosine * cosine));
        let direction = axis * cosine + (across * cos(azimuth) + depth * sin(azimuth)) * sine;
        let diskAngle = random.z * TAU;
        let radius = layer.spatial.y * sqrt(nextRandom(rng));
        let position = (across * cos(diskAngle) + depth * sin(diskAngle)) * radius;
        return InitialMotion(position, direction * speed);
    }
    let start = spawnPosition(random);
    return InitialMotion(vec3f(start.xy, 0.0), vec3f(cos(start.z), sin(start.z), 0.0) * speed);
}

fn newParticle(rng: ptr<function, u32>) -> Particle {
    let pos1 = nextRandom(rng);
    let pos2 = nextRandom(rng);
    let direction = nextRandom(rng);
    let life = mix(layer.lifeSpeed.x, layer.lifeSpeed.y, nextRandom(rng));
    let speed = mix(layer.lifeSpeed.z, layer.lifeSpeed.w, nextRandom(rng));
    let size = mix(layer.sizeRotation.x, layer.sizeRotation.y, nextRandom(rng));
    let rotation = mix(layer.sizeRotation.z, layer.sizeRotation.w, nextRandom(rng));
    let spin = mix(layer.spinGravity.x, layer.spinGravity.y, nextRandom(rng));
    // The reference reserves five draws for lifetime ranges and flipbooks.
    *rng += 0x21d961c9u;
    var phase = 0.0;
    if layer.noise.w != 0.0 {
        phase = nextRandom(rng);
    }
    var motion = spawnMotion(vec3f(pos1, pos2, direction), speed, rng);
    if layer.placement.z > 0.0 {
        motion.position.y -= max(0.0, frame.clock * (layer.placement.z - 0.5 * layer.placement.w * frame.clock));
    }
    return Particle(motion.position, phase, motion.velocity, spin, 0.0, life, size, rotation);
}

fn sampleAt(t: f32, component: u32) -> vec4f {
    let sampleIndex = clamp(t, 0.0, 1.0) * 255.0;
    let i = u32(sampleIndex);
    return mix(layer.samples[i * 2u + component],
        layer.samples[min(i + 1u, 255u) * 2u + component], fract(sampleIndex));
}

fn noiseHash(cell: vec2i, seed: u32) -> f32 {
    let x = bitcast<u32>(cell.x) * 0x9e3779b9u;
    let y = bitcast<u32>(cell.y) * 0x85ebca6bu;
    return randomWord((x ^ y ^ seed) + RNG_STEP);
}

fn noiseValue(position: vec2f, seed: u32) -> f32 {
    let cell = vec2i(floor(position));
    let f = fract(position);
    let blend = f * f * f * (f * (f * 6.0 - 15.0) + 10.0);
    let low = mix(noiseHash(cell, seed), noiseHash(cell + vec2i(1, 0), seed), blend.x);
    let high = mix(noiseHash(cell + vec2i(0, 1), seed), noiseHash(cell + vec2i(1, 1), seed), blend.x);
    return mix(low, high, blend.y);
}

fn curl(position: vec2f, seed: u32, octaves: u32) -> vec2f {
    var result = vec2f(0.0);
    var amplitude = 1.0;
    var point = position;
    let dx = vec2f(0.35, 0.0);
    let dy = dx.yx;
    for (var i = 0u; i < octaves; i++) {
        result += (amplitude / 0.7) * vec2f(
            noiseValue(point + dy, seed) - noiseValue(point - dy, seed),
            noiseValue(point - dx, seed) - noiseValue(point + dx, seed));
        point *= 2.0;
        amplitude *= 0.5;
    }
    return result;
}

@compute @workgroup_size(64)
fn advance(@builtin(global_invocation_id) id: vec3u) {
    if id.x >= arrayLength(&state.particles) {
        return;
    }
    if frame.reset != 0u {
        if id.x >= frame.reset - 1u {
            state.particles[id.x].life = 0.0;
        }
        return;
    }
    var particle = state.particles[id.x];
    if particle.life == 0.0 {
        return;
    }
    let age = particle.age + frame.dt;
    if age >= particle.life {
        state.particles[id.x].life = 0.0;
        return;
    }
    let tracks = sampleAt(particle.age / particle.life, 0u);
    let gravity = vec3f(layer.spinGravity.zw, 0.0);
    particle.velocity = (particle.velocity + gravity * frame.dt) * max(0.0, 1.0 - tracks.z * frame.dt);
    particle.position += particle.velocity * frame.dt;
    if layer.noise.w != 0.0 {
        let scroll = frame.clock * layer.noise.z * layer.noise.y;
        let volume = layer.spatial.x == SPHERE || layer.spatial.x == CONE;
        let phase = scroll + particle.noisePhase * 37.0 + select(0.0, particle.position.z * layer.noise.y, volume);
        let point = particle.position.xy * layer.noise.y + vec2f(phase, scroll * 0.773);
        let drift = curl(point, layer.seedSurfaceAspect.x + 0x9e3779b9u, u32(layer.noise.w));
        let depthDrift = select(0.0, drift.x * (particle.noisePhase * 2.0 - 1.0), volume);
        particle.position += vec3f(drift, depthDrift) * (tracks.w * frame.dt);
    }
    particle.rotation += (particle.spin + tracks.y) * frame.dt;
    particle.age = age;
    state.particles[id.x] = particle;
}

// One invocation preserves the reference RNG order and drops births when the
// pool is full. Motion runs in parallel in the preceding dispatch.
@compute @workgroup_size(1)
fn spawn() {
    var rng = select(state.rng, layer.seedSurfaceAspect.x, frame.reset == 1u);
    var remaining = frame.births;
    for (var i = 0u; i < arrayLength(&state.particles) && remaining != 0u; i++) {
        if state.particles[i].life == 0.0 {
            state.particles[i] = newParticle(&rng);
            remaining -= 1u;
        }
    }
    state.rng = rng;
}

struct Vertex {
    @builtin(position) position: vec4f,
    @location(0) uv: vec2f,
    @location(1) tint: vec4f,
}

struct ParticleCamera {
    viewProjection: mat4x4f,
    right: vec4f,
    up: vec4f,
}

@group(2) @binding(0) var<uniform> particleCamera: ParticleCamera;

const corners = array<vec2f, 6>(vec2f(0, 0), vec2f(1, 0), vec2f(0, 1), vec2f(0, 1), vec2f(1, 0), vec2f(1, 1));

fn particleCorner(corner: vec2f, attributes: vec4f, projectedVelocity: vec2f, speed: f32) -> vec2f {
    let t = attributes.x / attributes.y;
    let size = max(0.0, attributes.z * sampleAt(t, 0u).x);
    var xy = (corner - 0.5) * vec2f(size, size * bitcast<f32>(layer.seedSurfaceAspect.z));
    if layer.renderStyle.x > 0.0 {
        let projectedSpeed = length(projectedVelocity);
        let align = layer.renderStyle.x == 2.0;
        let stretchSpeed = select(speed, projectedSpeed, align);
        xy.x *= clamp(1.0 + layer.renderStyle.y * stretchSpeed, layer.renderStyle.z, layer.renderStyle.w);
        if align && projectedSpeed >= 0.001 {
            let heading = projectedVelocity / projectedSpeed;
            return vec2f(heading.x * xy.x - heading.y * xy.y, heading.y * xy.x + heading.x * xy.y);
        }
    }
    let angle = attributes.w * RAD;
    let c = cos(angle);
    let s = sin(angle);
    return vec2f(c * xy.x - s * xy.y, s * xy.x + c * xy.y);
}

fn particleOutput(position: vec4f, corner: vec2f, attributes: vec4f) -> Vertex {
    return Vertex(position, mix(layer.uv.xy, layer.uv.zw, corner), sampleAt(attributes.x / attributes.y, 1u));
}

fn hiddenParticle() -> Vertex {
    return Vertex(vec4f(2.0, 2.0, 0.0, 1.0), vec2f(0.0), vec4f(0.0));
}

@vertex
fn particleVertex(
    @builtin(vertex_index) vertexIndex: u32,
    @location(0) center: vec3f,
    @location(1) attributes: vec4f,
    @location(2) velocity: vec3f,
) -> Vertex {
    if attributes.y == 0.0 {
        return hiddenParticle();
    }

    let corner = corners[vertexIndex];
    var speed = 0.0;
    if layer.renderStyle.x > 0.0 {
        speed = length(velocity.xy);
    }
    let pixel = center.xy + particleCorner(corner, attributes, velocity.xy, speed) + frame.origin;
    let ndc = pixel / frame.extent * vec2f(2.0, -2.0) + vec2f(-1.0, 1.0);
    return particleOutput(vec4f(ndc, 0.0, 1.0), corner, attributes);
}

@vertex
fn worldParticleVertex(
    @builtin(vertex_index) vertexIndex: u32,
    @location(0) center: vec3f,
    @location(1) attributes: vec4f,
    @location(2) velocity: vec3f,
) -> Vertex {
    if attributes.y == 0.0 {
        return hiddenParticle();
    }

    let corner = corners[vertexIndex];
    let ground = layer.spatial.x == GROUND;
    var projected = vec2f(0.0);
    var speed = 0.0;
    if layer.renderStyle.x == 2.0 {
        let motion = select(velocity, velocity.xzy, ground) * vec3f(1.0, -1.0, 1.0);
        projected = vec2f(dot(motion, particleCamera.right.xyz), -dot(motion, particleCamera.up.xyz));
    } else if layer.renderStyle.x == 1.0 {
        speed = length(velocity);
    }
    let delta = particleCorner(corner, attributes, projected, speed);
    let billboard = particleCamera.right.xyz * delta.x - particleCamera.up.xyz * delta.y;
    let origin = vec3f(frame.origin, frame.extent.x);
    let local = select(center, center.xzy, ground);
    let placement = vec3f(layer.placement.x, layer.spatial.w, layer.placement.y);
    let world = origin + (vec3f(local.x, -local.y, local.z) + placement + billboard) * frame.extent.y;
    return particleOutput(particleCamera.viewProjection * vec4f(world, 1.0), corner, attributes);
}

@fragment
fn particleFragment(input: Vertex) -> @location(0) vec4f {
    let color = textureSample(page, pageSampler, input.uv) * input.tint;
    return vec4f(color.rgb * color.a, color.a);
}
