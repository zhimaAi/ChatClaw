import path from "node:path";
import { defineConfig } from "vite";
import vue from "@vitejs/plugin-vue";
import tailwindcss from "@tailwindcss/vite";
import wails from "@wailsio/runtime/plugins/vite";
import svgLoader from "vite-svg-loader";

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [vue(), tailwindcss(), wails("./bindings"), svgLoader({ defaultImport: 'component' })],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
      "@bindings": path.resolve(__dirname, "./bindings"),
    },
  },
  build: {
    rollupOptions: {
      input: {
        main: "index.html",
        winsnap: "winsnap.html",
        floatingball: "floatingball.html",
        selection: "selection.html",
        historyRun: "history-run.html",
      },
      output: {
        manualChunks(id) {
          if (id.includes("node_modules")) {
            if (id.includes("highlight.js")) return "vendor-hljs";
            if (id.includes("@vue-office") || id.includes("bestofdview")) return "vendor-office";
            if (id.includes("reka-ui")) return "vendor-reka";
            if (id.includes("lucide-vue-next")) return "vendor-icons";
            if (id.includes("marked") || id.includes("dompurify")) return "vendor-markdown";
            if (id.includes("vue-i18n")) return "vendor-i18n";
            if (id.includes("@vueuse")) return "vendor-vueuse";
            return "vendor";
          }
        },
      },
    },
  },
});
