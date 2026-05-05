import { defineConfig } from "vite";
import vue from "@vitejs/plugin-vue";

export default defineConfig({
  plugins: [vue()],
  build: {
    outDir: "dist",
    emptyOutDir: true,
    sourcemap: false,
    cssCodeSplit: true,
    chunkSizeWarningLimit: 1200,
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (!id.includes("node_modules")) return;
          if (id.includes("echarts")) return "vendor-echarts";
          if (id.includes("element-plus")) return "vendor-element-plus";
          if (id.includes("axios")) return "vendor-axios";
          return "vendor";
        }
      }
    }
  }
});
