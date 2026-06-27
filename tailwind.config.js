/** @type {import('tailwindcss').Config} */
module.exports = {
  content: [
    './internal/web/template/**/*.html',
    './internal/web/static/app.js',
  ],
  theme: {
    extend: {},
  },
  plugins: [require('daisyui')],
  daisyui: {
    themes: ['light'],
    logs: false,
  },
};
