import { defineConfig } from 'vite';
import { viteStaticCopy } from 'vite-plugin-static-copy';

const iconsPath = 'node_modules/@shoelace-style/shoelace/dist/assets/icons';

console.log(process.cwd());

// https://vitejs.dev/config/
export default defineConfig((env) => { return {
    base: env.command === 'build' ? '/lit/' : undefined,
    resolve: {
        alias: [
            {
                find: /\/assets\/icons\/(.+)/,
                replacement: `${iconsPath}/$1`,
            },
        ],
    },
    build: {
        rollupOptions: {
            // external: /^lit/,
            plugins: [],
        },
    },
    plugins: [
        viteStaticCopy({
            targets: [
                {
                    src: iconsPath,
                    dest: 'assets',
                },
            ],
        }),
    ],
    server: {
        proxy: {
            '/api': 'http://localhost:8080'
        }
    }
}});
