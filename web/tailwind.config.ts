import type { Config } from 'tailwindcss';

// Semantic colors come from the CSS variables in app/globals.css, which
// switch with the operating system's light/dark preference.
const token = (name: string) => `hsl(var(--${name}) / <alpha-value>)`;

const config: Config = {
  content: [
    './app/**/*.{js,ts,jsx,tsx,mdx}',
    './components/**/*.{js,ts,jsx,tsx,mdx}',
  ],
  darkMode: 'media',
  theme: {
    extend: {
      colors: {
        background: token('background'),
        foreground: token('foreground'),
        border: token('border'),
        ring: token('ring'),
        card: { DEFAULT: token('card'), foreground: token('card-foreground') },
        primary: { DEFAULT: token('primary'), foreground: token('primary-foreground') },
        secondary: { DEFAULT: token('secondary'), foreground: token('secondary-foreground') },
        muted: { DEFAULT: token('muted'), foreground: token('muted-foreground') },
        accent: { DEFAULT: token('accent'), foreground: token('accent-foreground') },
        // Compliance status colors
        compliance: {
          full: '#22c55e',
          partial: '#f59e0b',
          none: '#ef4444',
          na: '#6b7280',
        },
      },
    },
  },
  plugins: [],
};

export default config;
