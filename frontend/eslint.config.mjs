import nextCoreWebVitals from 'eslint-config-next/core-web-vitals'
import reactHooks from 'eslint-plugin-react-hooks'

const eslintConfig = [
  {
    ignores: ['.next/**', 'out/**', 'public/monaco-editor/**', 'next-env.d.ts', 'tsconfig.tsbuildinfo'],
  },
  ...nextCoreWebVitals,
  {
    plugins: {
      'react-hooks': reactHooks,
    },
    rules: {
      'react-hooks/immutability': 'warn',
      'react-hooks/incompatible-library': 'warn',
      'react-hooks/preserve-manual-memoization': 'warn',
      'react-hooks/purity': 'warn',
      'react-hooks/refs': 'warn',
      'react-hooks/set-state-in-effect': 'warn',
      'react-hooks/static-components': 'warn',
    },
  },
]

export default eslintConfig
