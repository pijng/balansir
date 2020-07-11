import resolve from '@rollup/plugin-node-resolve';
import commonjs from '@rollup/plugin-commonjs';
 
const config = {
  input: 'content/assets/js/src/index.js',
  output: {
    dir: 'content/assets/js/dist/',
    format: 'esm'
  },
  watch: {
    include: "content/assets/js/src/**"
  },
  plugins: [resolve(), commonjs()]
};
 
export default config;