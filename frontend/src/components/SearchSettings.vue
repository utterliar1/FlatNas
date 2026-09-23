<script setup lang="ts">
import { useMainStore } from "../stores/main";
import { VueDraggable } from "vue-draggable-plus";
import type { SearchEngine } from "@/types";

const store = useMainStore();

const addSearchEngine = () => {
  const id = Date.now().toString();
  const key = "custom-" + id;
  const label = "新搜索引擎";
  const urlTemplate = "https://example.com/search?q={q}";

  if (!store.appConfig.searchEngines) {
    store.appConfig.searchEngines = [];
  }
  store.appConfig.searchEngines.push({ id, key, label, urlTemplate });
};

const removeSearchEngine = (key: string) => {
  const list = (store.appConfig.searchEngines || []).filter((e: SearchEngine) => e.key !== key);
  store.appConfig.searchEngines = list;
  if (store.appConfig.defaultSearchEngine === key) {
    store.appConfig.defaultSearchEngine = list[0]?.key || "";
  }
};

// 上传并裁剪引擎图标：统一缩放居中裁剪为 64x64，减小存储体积
const onIconUpload = (event: Event, engine: SearchEngine) => {
  const input = event.target as HTMLInputElement;
  const file = input.files?.[0];
  input.value = "";
  if (!file) return;
  if (!file.type.startsWith("image/")) return;

  const reader = new FileReader();
  reader.onload = () => {
    const img = new Image();
    img.onload = () => {
      const size = 64;
      const canvas = document.createElement("canvas");
      canvas.width = size;
      canvas.height = size;
      const ctx = canvas.getContext("2d");
      if (!ctx) return;
      const scale = Math.max(size / img.width, size / img.height);
      const w = img.width * scale;
      const h = img.height * scale;
      ctx.drawImage(img, (size - w) / 2, (size - h) / 2, w, h);
      try {
        engine.icon = canvas.toDataURL("image/webp", 0.9);
      } catch {
        engine.icon = canvas.toDataURL("image/png");
      }
    };
    img.src = reader.result as string;
  };
  reader.readAsDataURL(file);
};

const removeIcon = (engine: SearchEngine) => {
  delete engine.icon;
};
</script>

<template>
  <div class="space-y-6">
    <h4 class="text-lg font-bold mb-2 text-gray-800 border-l-4 border-blue-500 pl-3">
      搜索引擎设置
    </h4>
    <div class="text-xs text-gray-500 mb-2">
      拖拽调整优先级；设置默认或开启“记住上次选择”。点击引擎图标可上传自定义图片。
    </div>
    <VueDraggable
      v-model="store.appConfig.searchEngines"
      :animation="300"
      :forceFallback="true"
      handle=".drag-handle"
      class="space-y-3"
      ghost-class="opacity-50"
      fallback-class="drag-fallback"
    >
      <div
        v-for="e in store.appConfig.searchEngines"
        :key="e.id"
        class="p-3 rounded-xl border border-gray-200 bg-gray-50 hover:bg-white transition-all flex flex-col gap-2"
      >
        <div class="flex items-center justify-between">
          <div class="flex items-center gap-2 flex-1">
            <div
              class="drag-handle cursor-grab active:cursor-grabbing text-gray-400 hover:text-gray-600 p-1 select-none"
            >
              <svg
                xmlns="http://www.w3.org/2000/svg"
                class="h-5 w-5"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  stroke-width="2"
                  d="M4 6h16M4 12h16M4 18h16"
                />
              </svg>
            </div>
            <label
              class="relative h-9 w-9 flex-shrink-0 cursor-pointer group"
              title="点击上传引擎图标"
            >
              <img
                v-if="e.icon"
                :src="e.icon"
                class="h-9 w-9 rounded-lg object-contain border border-gray-200 bg-white"
              />
              <div
                v-else
                class="h-9 w-9 rounded-lg border-2 border-dashed border-gray-300 bg-white flex items-center justify-center text-gray-300 text-sm font-bold group-hover:border-blue-400 group-hover:text-blue-400 transition-colors"
              >
                {{ e.label.slice(0, 1) }}
              </div>
              <span
                class="absolute inset-0 rounded-lg bg-black/40 text-white opacity-0 group-hover:opacity-100 flex items-center justify-center text-[10px] transition-opacity pointer-events-none"
                >更换</span
              >
              <span
                v-if="e.icon"
                role="button"
                aria-label="移除图标"
                class="absolute -top-1.5 -right-1.5 h-4 w-4 rounded-full bg-gray-500 hover:bg-red-500 text-white text-[9px] leading-none flex items-center justify-center cursor-pointer shadow"
                @click.stop.prevent="removeIcon(e)"
                >✕</span
              >
              <input
                type="file"
                accept="image/*"
                class="hidden"
                @change="onIconUpload($event, e)"
              />
            </label>
            <input
              v-model="e.label"
              class="font-bold text-gray-700 bg-transparent border-b border-transparent focus:border-blue-500 outline-none w-24"
            />
            <span class="text-xs text-gray-400 font-mono select-none">{{ e.key }}</span>
          </div>
          <div class="flex items-center gap-2">
            <label
              class="flex items-center gap-1 text-xs px-2 py-1 rounded-lg cursor-pointer transition-colors"
              :class="{
                'text-blue-600 font-bold bg-blue-50 border-blue-100':
                  store.appConfig.defaultSearchEngine === e.key,
              }"
            >
              <span>{{
                store.appConfig.defaultSearchEngine === e.key ? "当前默认" : "设为默认"
              }}</span>
              <input
                type="radio"
                :value="e.key"
                v-model="store.appConfig.defaultSearchEngine"
                class="accent-blue-500 w-3 h-3 cursor-pointer"
              />
            </label>
            <button
              class="text-xs text-red-500 hover:underline px-1"
              @click="removeSearchEngine(e.key)"
            >
              删除
            </button>
          </div>
        </div>
        <div class="flex items-center gap-2">
          <label class="text-[10px] text-gray-500">URL 模板</label>
          <input
            v-model="e.urlTemplate"
            class="flex-1 px-3 py-2 border border-gray-200 rounded-lg text-xs focus:border-blue-500 outline-none"
            placeholder="例如：https://example.com/search?q={q}"
          />
        </div>
      </div>
    </VueDraggable>
    <div class="flex items-center gap-3">
      <button
        @click="addSearchEngine"
        class="flex-1 p-2 border-2 border-dashed border-gray-200 rounded-xl text-gray-400 hover:border-blue-400 hover:text-blue-500 hover:bg-blue-50 transition-all text-sm font-bold"
      >
        + 添加搜索引擎
      </button>
    </div>
  </div>
</template>

<style scoped>
.drag-fallback {
  opacity: 1 !important;
  background: white;
  border: 1px solid #3b82f6;
  box-shadow: 0 10px 15px -3px rgba(0, 0, 0, 0.1), 0 4px 6px -2px rgba(0, 0, 0, 0.05);
  transform: scale(1.02);
  cursor: grabbing;
  z-index: 9999;
}
</style>
