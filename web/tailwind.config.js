/** @type {import('tailwindcss').Config} */
export default {
  content: [
    "./index.html",
    "./src/**/*.{js,ts,jsx,tsx}",
  ],
  theme: {
    extend: {
      colors: {
        bg: "#0d1117",
        surface: "#161b22",
        "surface-hover": "#21262d",
        border: "#30363d",
        text: "#e6edf3",
        muted: "#8b949e",
        accent: "#2f81f7",
        "accent-hover": "#1f6feb",
        green: "#2ea043",
        amber: "#d29922",
        red: "#f85149",
      },
      fontFamily: {
        sans: ["system-ui", "-apple-system", "Segoe UI", "Roboto", "sans-serif"],
      },
      animation: {
        "status-pulse": "status-pulse 1.5s cubic-bezier(0.4, 0, 0.6, 1) infinite",
        "glow-pulse": "glow-pulse 2s infinite",
      },
      keyframes: {
        "status-pulse": {
          "0%, 100%": { opacity: "1" },
          "50%": { opacity: ".3" },
        },
        "glow-pulse": {
          "0%, 100%": { boxShadow: "0 0 5px rgba(47, 129, 247, 0.2)" },
          "50%": { boxShadow: "0 0 15px rgba(47, 129, 247, 0.6)" },
        },
      },
    },
  },
  plugins: [],
}
