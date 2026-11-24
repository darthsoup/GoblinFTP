import antfu from '@antfu/eslint-config'

export default antfu({
  vue: true,
  typescript: true,
  formatters: true,
}, {
  rules: {
    'antfu/reasonable-newline': 'off',
    // antfu defaults to script-first; we read templates first.
    'vue/block-order': ['error', { order: ['template', 'script', 'style'] }],
  },
})
