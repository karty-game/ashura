let decoder;
self.onmessage = async ({data}) => {
    try {
        if (data.wasm) {
            decoder = await BASIS({wasmBinary: data.wasm});
            decoder.initializeBasis();
            self.postMessage({ready: true});
            return;
        }
        const file = new decoder.KTX2File(data.input);
        try {
            if (!file.isValid() || !file.isETC1S() || file.getHasAlpha() || !file.isSRGB() || !file.startTranscoding()) throw Error('invalid ETC1S albedo');
            const levels = [];
            for (let level = 0; level < file.getLevels(); level++) {
                const bytes = file.getImageTranscodedSizeInBytes(level, 0, 0, data.format);
                const output = new Uint8Array(bytes);
                if (!file.transcodeImage(output, level, 0, 0, data.format, 0, -1, -1)) throw Error('ETC1S block transcode failed');
                levels.push(output);
            }
            self.postMessage({width: file.getWidth(), height: file.getHeight(), levels}, levels.map(level => level.buffer));
        } finally { file.close(); file.delete(); }
    } catch (error) { self.postMessage({error: String(error)}); }
};
