/** @type {import('tailwindcss').Config} */
module.exports = {
  darkMode: 'class',
  content: ['./src/**/*.{js,jsx,ts,tsx}'],
  theme: {
    extend: {
      colors: {
        primary: {
          DEFAULT: '#0097a7',
          light: '#4dd0e1',
          dark: '#006064',
          foreground: '#ffffff',
        },
        secondary: {
          DEFAULT: '#26a69a',
          light: '#80cbc4',
          dark: '#00796b',
          foreground: '#ffffff',
        },
        background: {
          DEFAULT: '#f0fafa',
          paper: '#ffffff',
          'dark-default': '#071e26',
          'dark-paper': '#0d2f3a',
        },
        foreground: {
          DEFAULT: '#0d3c47',
          secondary: '#37737d',
          'dark-default': '#e0f7fa',
          'dark-secondary': '#80cbc4',
        },
      },
      fontFamily: {
        sans: ['Inter', 'Roboto', 'Helvetica', 'Arial', 'sans-serif'],
      },
      borderRadius: {
        DEFAULT: '10px',
      },
    },
  },
  plugins: [require('@tailwindcss/typography')],
};
