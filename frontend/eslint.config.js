import js from '@eslint/js';

export default [
  js.configs.recommended,
  {
    languageOptions: {
      ecmaVersion: 'latest',
      sourceType: 'module',
      globals: {
        '__dirname': 'readonly',
        process: 'readonly',
        console: 'readonly',
        setTimeout: 'readonly',
        setImmediate: 'readonly',
        setInterval: 'readonly',
        clearTimeout: 'readonly',
        clearInterval: 'readonly',
        window: 'readonly',
        document: 'readonly',
        navigator: 'readonly',
        WebSocket: 'readonly',
      },
    },
    rules: {
      'no-unused-vars': ['warn', {
        argsIgnorePattern: '^_',
        varsIgnorePattern: '^_',
      }],
      'no-console': 'off',
      'no-undef': 'error',
      'comma-dangle': ['error', 'always-multiline'],
      quotes: ['error', 'single', { avoidEscape: true }],
    },
  },
  {
    ignores: [
      '**/node_modules/',
      '**/dist/',
      '**/build/',
      '**/*.min.js',
      '**/coverage/',
      '**/.git/',
      '**/backup/',
      '**/hls/',
    ],
  },
];
