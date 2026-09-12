/** @type {import('tailwindcss').Config} */

// Semantic colors are backed by CSS custom properties (see index.css) so the
// whole app can switch between the dark (default) and light themes from one
// place. Each var holds an "R G B" channel triple, exposed through the
// rgb(var() / <alpha-value>) pattern so Tailwind opacity modifiers
// (e.g. bg-surface-2/50) keep working.
const v = (name) => `rgb(var(${name}) / <alpha-value>)`;

export default {
  content: [
    "./index.html",
    "./src/**/*.{js,ts,jsx,tsx}",
  ],
  darkMode: ['class', '[data-theme="dark"]'],
  theme: {
    extend: {
      colors: {
        // Accent ramp (sky). Kept static so focus rings/brand stay consistent;
        // `primary` is the canonical name used across the app.
        primary: {
          50: '#f0f9ff',
          100: '#e0f2fe',
          200: '#bae6fd',
          300: '#7dd3fc',
          400: '#38bdf8',
          500: '#0ea5e9',
          600: '#0284c7',
          700: '#0369a1',
          800: '#075985',
          900: '#0c4a6e',
        },

        // Semantic surface/typography roles (theme-aware).
        app: v('--c-app'),
        surface: {
          DEFAULT: v('--c-surface'),
          2: v('--c-surface-2'),
          3: v('--c-surface-3'),
        },
        line: {
          DEFAULT: v('--c-line'),
          strong: v('--c-line-strong'),
        },
        fg: {
          DEFAULT: v('--c-fg'),
          2: v('--c-fg-2'),
          3: v('--c-fg-3'),
        },
        muted: {
          DEFAULT: v('--c-muted'),
          2: v('--c-muted-2'),
        },

        // Status roles (theme-aware). Use /15 etc. for tinted backgrounds.
        ok: v('--c-ok'),
        warn: v('--c-warn'),
        danger: v('--c-danger'),
        info: v('--c-info'),
      },
    },
  },
  plugins: [],
}
