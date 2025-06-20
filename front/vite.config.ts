import { defineConfig } from 'vite';
import solidPlugin from 'vite-plugin-solid';
import { viteSingleFile } from 'vite-plugin-singlefile';

export default defineConfig({
    plugins: process.env.VITEST ? [] : [solidPlugin(), viteSingleFile()],
    server: {
        port: 8081,
    },
    build: {
        target: 'esnext',
    },
});
