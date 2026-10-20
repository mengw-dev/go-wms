import js from '@eslint/js'
import pluginVue from 'eslint-plugin-vue'
import tseslint from 'typescript-eslint'
import businessDemoBoundary from './eslint-rules/business-demo-boundary.mjs'

export default tseslint.config(
  { ignores: ['dist/**', 'node_modules/**', 'coverage/**', 'playwright-report/**', 'test-results/**'] },
  js.configs.recommended,
  ...tseslint.configs.recommended,
  ...pluginVue.configs['flat/essential'],
  {
    files: ['src/**/*.{js,mjs,cjs,jsx,ts,mts,cts,tsx,vue}'],
    plugins: { architecture: { rules: { 'business-demo-boundary': businessDemoBoundary } } },
    rules: { 'architecture/business-demo-boundary': 'error' },
  },
  {
    languageOptions: {
      globals: {
        window: 'readonly',
        document: 'readonly',
        localStorage: 'readonly',
        sessionStorage: 'readonly',
        File: 'readonly',
        FormData: 'readonly',
        Blob: 'readonly',
        crypto: 'readonly',
        // .vue <script> 中引用的 DOM 类型（.ts 文件由 tseslint 豁免 no-undef，.vue 不会）
        HTMLElement: 'readonly',
        KeyboardEvent: 'readonly',
      },
    },
  },
  {
    files: ['**/*.vue'],
    languageOptions: {
      parserOptions: {
        parser: tseslint.parser,
        extraFileExtensions: ['.vue'],
      },
    },
  },
  {
    files: ['tests/**/*.ts', 'playwright.config.ts'],
    languageOptions: {
      globals: {
        process: 'readonly',
      },
    },
  },
  {
    rules: {
      'vue/multi-word-component-names': 'off',
    },
  },
)
