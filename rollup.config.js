import resolve from '@rollup/plugin-node-resolve';
import commonjs from '@rollup/plugin-commonjs';
import { terser } from 'rollup-plugin-terser';
import babel from '@rollup/plugin-babel';

const DEVELOPMENT = process.env.DEVELOPMENT

const config = {
  input: 'content/assets/js/src/index.js',
  output: {
    dir: 'content/assets/js/dist/',
    sourcemap: DEVELOPMENT,
    format: 'iife',
  },
  watch: {
    include: "content/assets/js/src/**",
  },
  plugins: [
    resolve({
      browser: true
    }),
    babel({
      exclude: 'node_modules/**',
      babelHelpers: 'bundled',
    }),
    commonjs(),
  ],
}

if (!DEVELOPMENT) {
  config.plugins.push(terser())
}

export default config;