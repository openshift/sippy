import js from '@eslint/js'
import globals from 'globals'
import babelParser from '@babel/eslint-parser'
import reactPlugin from 'eslint-plugin-react'
import prettierPlugin from 'eslint-plugin-prettier/recommended'
import sortImports from 'eslint-plugin-sort-imports-es6-autofix'
import unusedImports from 'eslint-plugin-unused-imports'
import vitestPlugin from '@vitest/eslint-plugin'

export default [
  {
    ignores: ['node_modules/**', 'build/**', 'coverage/**'],
  },
  js.configs.recommended,
  {
    settings: {
      react: {
        version: 'detect',
      },
    },
  },
  reactPlugin.configs.flat.recommended,
  prettierPlugin,
  {
    files: ['**/*.{js,jsx}'],
    languageOptions: {
      parser: babelParser,
      parserOptions: {
        requireConfigFile: false,
        babelOptions: {
          presets: ['@babel/preset-env', '@babel/preset-react'],
        },
        ecmaFeatures: {
          jsx: true,
        },
        ecmaVersion: 2021,
        sourceType: 'module',
      },
      globals: {
        ...globals.browser,
      },
    },
    plugins: {
      'sort-imports-es6-autofix': sortImports,
      'unused-imports': unusedImports,
    },
    rules: {
      'no-unused-vars': [
        'error',
        {
          argsIgnorePattern: '^_',
          varsIgnorePattern: '^_',
          caughtErrorsIgnorePattern: '^_',
        },
      ],
      'react/display-name': 'off',
      'react/jsx-uses-react': 'warn',
      'react/jsx-uses-vars': 'warn',
      'react/no-unknown-property': 'off',
      'unused-imports/no-unused-imports': 'error',
      'unused-imports/no-unused-vars': 'off',
      'prettier/prettier': ['error'],
      'sort-imports-es6-autofix/sort-imports-es6': [
        'error',
        {
          ignoreCase: true,
          ignoreMemberSort: false,
          memberSyntaxSortOrder: ['none', 'all', 'multiple', 'single'],
        },
      ],
    },
  },
  {
    files: [
      '**/*.test.{js,jsx}',
      '**/*.spec.{js,jsx}',
      '**/setupTests.{js,jsx}',
    ],
    ...vitestPlugin.configs.recommended,
  },
  {
    files: [
      '**/*.test.{js,jsx}',
      '**/*.spec.{js,jsx}',
      '**/setupTests.{js,jsx}',
    ],
    languageOptions: {
      globals: {
        ...vitestPlugin.configs.env.languageOptions.globals,
        ...globals.node,
      },
    },
  },
]
