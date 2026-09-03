/** @type {import('tailwindcss').Config} */
export default {
  content: [
    "./index.html",
    "./src/**/*.{js,ts,jsx,tsx}",
  ],
  theme: {
    extend: {
      colors: {
        coldharbor: {
          900: '#0b0f19',
          850: '#111827',
          800: '#1f2937',
          700: '#374151',
          accent: '#06b6d4', // cyan-500
          innie: '#6366f1',  // indigo-500
          outie: '#10b981',  // emerald-500
          danger: '#ef4444', // red-500
          warning: '#f59e0b',// amber-500
        }
      },
      fontFamily: {
        mono: ['ui-monospace', 'SFMono-Regular', 'Menlo', 'Monaco', 'Consolas', 'monospace'],
      }
    },
  },
  plugins: [],
}
