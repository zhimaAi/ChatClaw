// vite.config.ts
import path from "node:path";
import { defineConfig } from "file:///usr/local/var/www/willchat-client/frontend/node_modules/vite/dist/node/index.js";
import vue from "file:///usr/local/var/www/willchat-client/frontend/node_modules/@vitejs/plugin-vue/dist/index.mjs";
import tailwindcss from "file:///usr/local/var/www/willchat-client/frontend/node_modules/@tailwindcss/vite/dist/index.mjs";
import wails from "file:///usr/local/var/www/willchat-client/frontend/node_modules/@wailsio/runtime/dist/plugins/vite.js";
import svgLoader from "file:///usr/local/var/www/willchat-client/frontend/node_modules/vite-svg-loader/index.js";
var __vite_injected_original_dirname = "/usr/local/var/www/willchat-client/frontend";
var vite_config_default = defineConfig({
  plugins: [vue(), tailwindcss(), wails("./bindings"), svgLoader({ defaultImport: "component" })],
  resolve: {
    alias: {
      "@": path.resolve(__vite_injected_original_dirname, "./src"),
      "@bindings": path.resolve(__vite_injected_original_dirname, "./bindings")
    }
  },
  build: {
    rollupOptions: {
      input: {
        main: "index.html",
        winsnap: "winsnap.html"
      }
    }
  }
});
export {
  vite_config_default as default
};
//# sourceMappingURL=data:application/json;base64,ewogICJ2ZXJzaW9uIjogMywKICAic291cmNlcyI6IFsidml0ZS5jb25maWcudHMiXSwKICAic291cmNlc0NvbnRlbnQiOiBbImNvbnN0IF9fdml0ZV9pbmplY3RlZF9vcmlnaW5hbF9kaXJuYW1lID0gXCIvdXNyL2xvY2FsL3Zhci93d3cvd2lsbGNoYXQtY2xpZW50L2Zyb250ZW5kXCI7Y29uc3QgX192aXRlX2luamVjdGVkX29yaWdpbmFsX2ZpbGVuYW1lID0gXCIvdXNyL2xvY2FsL3Zhci93d3cvd2lsbGNoYXQtY2xpZW50L2Zyb250ZW5kL3ZpdGUuY29uZmlnLnRzXCI7Y29uc3QgX192aXRlX2luamVjdGVkX29yaWdpbmFsX2ltcG9ydF9tZXRhX3VybCA9IFwiZmlsZTovLy91c3IvbG9jYWwvdmFyL3d3dy93aWxsY2hhdC1jbGllbnQvZnJvbnRlbmQvdml0ZS5jb25maWcudHNcIjtpbXBvcnQgcGF0aCBmcm9tIFwibm9kZTpwYXRoXCI7XG5pbXBvcnQgeyBkZWZpbmVDb25maWcgfSBmcm9tIFwidml0ZVwiO1xuaW1wb3J0IHZ1ZSBmcm9tIFwiQHZpdGVqcy9wbHVnaW4tdnVlXCI7XG5pbXBvcnQgdGFpbHdpbmRjc3MgZnJvbSBcIkB0YWlsd2luZGNzcy92aXRlXCI7XG5pbXBvcnQgd2FpbHMgZnJvbSBcIkB3YWlsc2lvL3J1bnRpbWUvcGx1Z2lucy92aXRlXCI7XG5pbXBvcnQgc3ZnTG9hZGVyIGZyb20gXCJ2aXRlLXN2Zy1sb2FkZXJcIjtcblxuLy8gaHR0cHM6Ly92aXRlanMuZGV2L2NvbmZpZy9cbmV4cG9ydCBkZWZhdWx0IGRlZmluZUNvbmZpZyh7XG4gIHBsdWdpbnM6IFt2dWUoKSwgdGFpbHdpbmRjc3MoKSwgd2FpbHMoXCIuL2JpbmRpbmdzXCIpLCBzdmdMb2FkZXIoeyBkZWZhdWx0SW1wb3J0OiAnY29tcG9uZW50JyB9KV0sXG4gIHJlc29sdmU6IHtcbiAgICBhbGlhczoge1xuICAgICAgXCJAXCI6IHBhdGgucmVzb2x2ZShfX2Rpcm5hbWUsIFwiLi9zcmNcIiksXG4gICAgICBcIkBiaW5kaW5nc1wiOiBwYXRoLnJlc29sdmUoX19kaXJuYW1lLCBcIi4vYmluZGluZ3NcIiksXG4gICAgfSxcbiAgfSxcbiAgYnVpbGQ6IHtcbiAgICByb2xsdXBPcHRpb25zOiB7XG4gICAgICBpbnB1dDoge1xuICAgICAgICBtYWluOiBcImluZGV4Lmh0bWxcIixcbiAgICAgICAgd2luc25hcDogXCJ3aW5zbmFwLmh0bWxcIixcbiAgICAgIH0sXG4gICAgfSxcbiAgfSxcbn0pO1xuIl0sCiAgIm1hcHBpbmdzIjogIjtBQUFtVCxPQUFPLFVBQVU7QUFDcFUsU0FBUyxvQkFBb0I7QUFDN0IsT0FBTyxTQUFTO0FBQ2hCLE9BQU8saUJBQWlCO0FBQ3hCLE9BQU8sV0FBVztBQUNsQixPQUFPLGVBQWU7QUFMdEIsSUFBTSxtQ0FBbUM7QUFRekMsSUFBTyxzQkFBUSxhQUFhO0FBQUEsRUFDMUIsU0FBUyxDQUFDLElBQUksR0FBRyxZQUFZLEdBQUcsTUFBTSxZQUFZLEdBQUcsVUFBVSxFQUFFLGVBQWUsWUFBWSxDQUFDLENBQUM7QUFBQSxFQUM5RixTQUFTO0FBQUEsSUFDUCxPQUFPO0FBQUEsTUFDTCxLQUFLLEtBQUssUUFBUSxrQ0FBVyxPQUFPO0FBQUEsTUFDcEMsYUFBYSxLQUFLLFFBQVEsa0NBQVcsWUFBWTtBQUFBLElBQ25EO0FBQUEsRUFDRjtBQUFBLEVBQ0EsT0FBTztBQUFBLElBQ0wsZUFBZTtBQUFBLE1BQ2IsT0FBTztBQUFBLFFBQ0wsTUFBTTtBQUFBLFFBQ04sU0FBUztBQUFBLE1BQ1g7QUFBQSxJQUNGO0FBQUEsRUFDRjtBQUNGLENBQUM7IiwKICAibmFtZXMiOiBbXQp9Cg==
