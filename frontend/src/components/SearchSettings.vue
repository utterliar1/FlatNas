<script setup lang="ts">
import { useI18n } from "vue-i18n";
import { useMainStore } from "../stores/main";
import { VueDraggable } from "vue-draggable-plus";
import type { SearchEngine } from "@/types";

const { t } = useI18n();
const store = useMainStore();

const addSearchEngine = () => {
  const id = Date.now().toString();
  const key = "custom-" + id;
  // 数据标识：写入用户数据并持久化，禁止 i18n
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
</script>

<template>
  <div class="space-y-6">
    <h4 class="text-lg font-bold mb-2 text-gray-800 border-l-4 border-blue-500 pl-3">
      {{ t("settings.searchSettings.title") }}
    </h4>
    <div class="text-xs text-gray-500 mb-2">
      {{ t("settings.searchSettings.desc") }}
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
                store.appConfig.defaultSearchEngine === e.key
                  ? t("settings.searchSettings.currentDefault")
                  : t("settings.searchSettings.setAsDefault")
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
              {{ t("common.common.delete") }}
            </button>
          </div>
        </div>
        <div class="flex items-center gap-2">
          <label class="text-[10px] text-gray-500">{{ t("settings.searchSettings.urlTemplate") }}</label>
          <input
            v-model="e.urlTemplate"
            class="flex-1 px-3 py-2 border border-gray-200 rounded-lg text-xs focus:border-blue-500 outline-none"
            :placeholder="t('settings.searchSettings.urlTemplatePlaceholder')"
          />
        </div>
      </div>
    </VueDraggable>
    <div class="flex items-center gap-3">
      <button
        @click="addSearchEngine"
        class="flex-1 p-2 border-2 border-dashed border-gray-200 rounded-xl text-gray-400 hover:border-blue-400 hover:text-blue-500 hover:bg-blue-50 transition-all text-sm font-bold"
      >
        {{ t("settings.searchSettings.addEngine") }}
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
