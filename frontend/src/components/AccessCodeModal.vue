<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { useMainStore } from "../stores/main";
import OverlayMotion from "@/components/base/OverlayMotion.vue";

const { t } = useI18n();

// 访问码弹窗：由「连点页面标题 3 次」触发的隐蔽入口。
//  - 未解锁：输入全局访问码解锁（后端校验并种会话 Cookie）；
//  - 已解锁：提供「重新上锁」；
//  - 管理员且尚未设置访问码：在此完成首次设置（输入两次）。
const props = defineProps<{
  show: boolean;
}>();

const emit = defineEmits(["update:show", "changed"]);
const store = useMainStore();

const UNLOCK_KEY = "flatnas_access_unlocked";

const unlocked = ref(false);
const code = ref("");
const confirmCode = ref("");
const error = ref("");
const busy = ref(false);
const shake = ref(false);

const isAdmin = computed(() => store.username === "admin" && store.isLogged);
const codeSet = computed(() => !!store.systemConfig.hasAccessCode);
// 管理员且未设置访问码 => 首次设置模式
const setupMode = computed(() => isAdmin.value && !codeSet.value);
// 管理员且已设置访问码 => 可在弹窗内切换为「管理」态（改码 / 清码）
const canManage = computed(() => isAdmin.value && codeSet.value);
const manageMode = ref(false);

// 解锁有效期选项（小时；0 = 会话内，关闭浏览器即上锁）
// 用 computed 包一层，切换语言时 label 才会跟着重新求值
const TTL_OPTIONS = computed(() => [
  { value: 0, label: t("settings.accessCode.ttlSession") },
  { value: 1, label: t("settings.accessCode.ttl1h") },
  { value: 12, label: t("settings.accessCode.ttl12h") },
  { value: 24, label: t("settings.accessCode.ttl24h") },
  { value: 168, label: t("settings.accessCode.ttl7d") },
]);
const unlockTTL = ref<number>(0);
const ttlSaving = ref(false);

watch(
  () => props.show,
  (v) => {
    if (v) {
      // 已解锁状态以服务端实时判定为准（accessUnlocked 依据解锁 Cookie 计算），
      // sessionStorage 标记仅作回退（老版本服务端无该字段时）。
      unlocked.value =
        store.systemConfig.accessUnlocked === true ||
        (typeof store.systemConfig.accessUnlocked === "undefined" &&
          sessionStorage.getItem(UNLOCK_KEY) === "1");
      unlockTTL.value = Number(store.systemConfig.accessUnlockTTL ?? 0);
      code.value = "";
      confirmCode.value = "";
      error.value = "";
      shake.value = false;
      manageMode.value = false;
    }
  },
);

const close = () => emit("update:show", false);

const showError = (msg: string) => {
  error.value = msg;
  shake.value = true;
  setTimeout(() => (shake.value = false), 400);
};

const handleUnlock = async () => {
  if (busy.value) return;
  if (!code.value.trim()) {
    showError(t("settings.accessCode.errEnterCode"));
    return;
  }
  busy.value = true;
  try {
    const res = await fetch("/api/access/unlock", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ code: code.value.trim() }),
    });
    if (res.ok) {
      sessionStorage.setItem(UNLOCK_KEY, "1");
      unlocked.value = true;
      emit("changed");
      close();
    } else {
      const data = await res.json().catch(() => ({}));
      showError(data.error || t("settings.accessCode.errWrongCode"));
    }
  } catch {
    showError(t("settings.accessCode.errNetwork"));
  } finally {
    busy.value = false;
  }
};

const handleLock = async () => {
  if (busy.value) return;
  busy.value = true;
  try {
    await fetch("/api/access/lock", { method: "POST" });
    sessionStorage.removeItem(UNLOCK_KEY);
    unlocked.value = false;
    emit("changed");
    close();
  } finally {
    busy.value = false;
  }
};

// 管理员修改解锁有效期（即时保存；后端会把 TTL 纳入 Cookie 签名，旧解锁立即按新策略失效）
const handleTTLChange = async () => {
  if (ttlSaving.value) return;
  ttlSaving.value = true;
  try {
    const ok = await store.updateSystemConfig({ accessUnlockTTL: unlockTTL.value });
    if (ok) {
      emit("changed");
    } else {
      showError(t("settings.accessCode.errUpdateFailed"));
    }
  } finally {
    ttlSaving.value = false;
  }
};

const handleSetup = async () => {
  if (busy.value) return;
  const c = code.value.trim();
  if (!c) {
    showError(t("settings.accessCode.errEnterCode"));
    return;
  }
  if (c.length < 4) {
    showError(t("settings.accessCode.errTooShort"));
    return;
  }
  if (c !== confirmCode.value.trim()) {
    showError(t("settings.accessCode.errMismatch"));
    return;
  }
  busy.value = true;
  try {
    const ok = await store.updateSystemConfig({ accessCode: c, accessUnlockTTL: unlockTTL.value });
    if (ok) {
      emit("changed");
      close();
    } else {
      showError(t("settings.accessCode.errSetupFailed"));
    }
  } finally {
    busy.value = false;
  }
};

// 清除全局访问码（关闭保护）：所有「访问码保护」分组将恢复为普通可见分组。
const handleClearCode = async () => {
  if (busy.value) return;
  if (!confirm(t("settings.accessCode.confirmClear"))) return;
  busy.value = true;
  try {
    const ok = await store.updateSystemConfig({ accessCode: "" });
    if (ok) {
      emit("changed");
      manageMode.value = false;
      close();
    } else {
      showError(t("settings.accessCode.errClearFailed"));
    }
  } finally {
    busy.value = false;
  }
};
</script>

<template>
  <OverlayMotion
    :show="show"
    :z-index="80"
    close-on-overlay
    overlay-class="bg-black/40 backdrop-blur-sm p-4"
    panel-class="max-w-xs"
    @close="close"
  >
    <div
      class="bg-white rounded-2xl shadow-2xl w-full overflow-hidden transition-transform"
      :class="{ 'animate-shake': shake }"
    >
      <!-- Header：刻意低调，伪装成普通密码确认框 -->
      <div class="px-5 pt-5 pb-1 flex flex-col items-center">
        <div
          class="w-10 h-10 rounded-full bg-gray-100 flex items-center justify-center text-gray-500 mb-2"
        >
          <svg
            xmlns="http://www.w3.org/2000/svg"
            viewBox="0 0 24 24"
            fill="currentColor"
            class="w-5 h-5"
          >
            <path
              fill-rule="evenodd"
              d="M12 1.5a5.25 5.25 0 00-5.25 5.25v3a3 3 0 00-3 3v6.75a3 3 0 003 3h10.5a3 3 0 003-3v-6.75a3 3 0 00-3-3v-3c0-2.9-2.35-5.25-5.25-5.25zm3.75 8.25v-3a3.75 3.75 0 10-7.5 0v3h7.5z"
              clip-rule="evenodd"
            />
          </svg>
        </div>
        <div class="text-sm font-bold text-gray-700">
          {{
            manageMode
              ? t("settings.accessCode.titleManage")
              : unlocked
                ? t("settings.accessCode.titleUnlocked")
                : setupMode
                  ? t("settings.accessCode.titleSetup")
                  : t("settings.accessCode.titleUnlock")
          }}
        </div>
      </div>

      <!-- Body -->
      <div class="px-5 pb-5 pt-3 space-y-3">
        <!-- 管理员管理访问码（修改 / 清除） -->
        <template v-if="manageMode">
          <p class="text-[11px] text-gray-400 text-center leading-relaxed">
            {{ t("settings.accessCode.manageHint") }}
          </p>
          <input
            v-model="code"
            type="password"
            autocomplete="off"
            :placeholder="t('settings.accessCode.newPlaceholder')"
            class="w-full px-3 py-2 border border-gray-200 rounded-xl text-sm outline-none focus:border-gray-800 text-center tracking-widest"
            @keyup.enter="handleSetup"
          />
          <input
            v-model="confirmCode"
            type="password"
            autocomplete="off"
            :placeholder="t('settings.accessCode.confirmPlaceholder')"
            class="w-full px-3 py-2 border border-gray-200 rounded-xl text-sm outline-none focus:border-gray-800 text-center tracking-widest"
            @keyup.enter="handleSetup"
          />
          <div class="space-y-1">
            <label class="block text-[11px] text-gray-500 px-1">{{
              t("settings.accessCode.ttlLabel")
            }}</label>
            <select
              v-model.number="unlockTTL"
              class="w-full px-3 py-2 border border-gray-200 rounded-xl text-sm outline-none focus:border-gray-800 bg-white"
            >
              <option v-for="opt in TTL_OPTIONS" :key="opt.value" :value="opt.value">
                {{ opt.label }}
              </option>
            </select>
          </div>
          <button
            @click="handleSetup"
            :disabled="busy"
            class="w-full py-2 rounded-xl text-sm font-bold text-white bg-gray-800 hover:bg-black transition-colors disabled:opacity-50"
          >
            {{ t("settings.accessCode.saveNew") }}
          </button>
          <button
            @click="handleClearCode"
            :disabled="busy"
            class="w-full py-2 rounded-xl text-sm font-bold text-red-600 bg-red-50 hover:bg-red-100 transition-colors disabled:opacity-50"
          >
            {{ t("settings.accessCode.clear") }}
          </button>
          <button
            @click="manageMode = false"
            class="w-full text-[11px] text-gray-400 hover:text-gray-600 transition-colors"
          >
            {{ t("settings.accessCode.back") }}
          </button>
        </template>

        <!-- 已解锁：提供重新上锁 -->
        <template v-if="!manageMode && unlocked">
          <p class="text-[11px] text-gray-400 text-center leading-relaxed">
            {{ t("settings.accessCode.unlockedHint") }}
          </p>
          <div v-if="isAdmin" class="space-y-1">
            <label class="block text-[11px] text-gray-500 px-1">{{
              t("settings.accessCode.ttlLabel")
            }}</label>
            <select
              v-model.number="unlockTTL"
              :disabled="ttlSaving"
              class="w-full px-3 py-2 border border-gray-200 rounded-xl text-sm outline-none focus:border-gray-800 bg-white disabled:opacity-50"
              @change="handleTTLChange"
            >
              <option v-for="opt in TTL_OPTIONS" :key="opt.value" :value="opt.value">
                {{ opt.label }}
              </option>
            </select>
          </div>
          <button
            @click="handleLock"
            :disabled="busy"
            class="w-full py-2 rounded-xl text-sm font-bold text-white bg-gray-800 hover:bg-black transition-colors disabled:opacity-50"
          >
            {{ t("settings.accessCode.relock") }}
          </button>
        </template>

        <!-- 管理员首次设置 -->
        <template v-else-if="!manageMode && setupMode">
          <p class="text-[11px] text-gray-400 text-center leading-relaxed">
            {{ t("settings.accessCode.setupHint") }}
          </p>
          <input
            v-model="code"
            type="password"
            autocomplete="off"
            :placeholder="t('settings.accessCode.setupPlaceholder')"
            class="w-full px-3 py-2 border border-gray-200 rounded-xl text-sm outline-none focus:border-gray-800 text-center tracking-widest"
            @keyup.enter="handleSetup"
          />
          <input
            v-model="confirmCode"
            type="password"
            autocomplete="off"
            :placeholder="t('settings.accessCode.confirmPlaceholder')"
            class="w-full px-3 py-2 border border-gray-200 rounded-xl text-sm outline-none focus:border-gray-800 text-center tracking-widest"
            @keyup.enter="handleSetup"
          />
          <div class="space-y-1">
            <label class="block text-[11px] text-gray-500 px-1">{{
              t("settings.accessCode.ttlLabel")
            }}</label>
            <select
              v-model.number="unlockTTL"
              class="w-full px-3 py-2 border border-gray-200 rounded-xl text-sm outline-none focus:border-gray-800 bg-white"
            >
              <option v-for="opt in TTL_OPTIONS" :key="opt.value" :value="opt.value">
                {{ opt.label }}
              </option>
            </select>
          </div>
          <button
            @click="handleSetup"
            :disabled="busy"
            class="w-full py-2 rounded-xl text-sm font-bold text-white bg-gray-800 hover:bg-black transition-colors disabled:opacity-50"
          >
            {{ t("settings.accessCode.save") }}
          </button>
        </template>

        <!-- 常规输码解锁 -->
        <template v-else-if="!manageMode">
          <input
            v-model="code"
            type="password"
            autocomplete="off"
            :placeholder="t('settings.accessCode.codePlaceholder')"
            class="w-full px-3 py-2 border border-gray-200 rounded-xl text-sm outline-none focus:border-gray-800 text-center tracking-widest"
            @keyup.enter="handleUnlock"
          />
          <button
            @click="handleUnlock"
            :disabled="busy"
            class="w-full py-2 rounded-xl text-sm font-bold text-white bg-gray-800 hover:bg-black transition-colors disabled:opacity-50"
          >
            {{ t("common.common.confirm") }}
          </button>
        </template>

        <button
          v-if="canManage && !manageMode"
          @click="manageMode = true"
          class="w-full text-[11px] text-gray-400 hover:text-gray-600 transition-colors"
        >
          {{ t("settings.accessCode.titleManage") }}
        </button>

        <p v-if="error" class="text-[11px] text-red-500 text-center">{{ error }}</p>
      </div>
    </div>
  </OverlayMotion>
</template>

<style scoped>
@keyframes shake {
  0%,
  100% {
    transform: translateX(0);
  }
  20%,
  60% {
    transform: translateX(-6px);
  }
  40%,
  80% {
    transform: translateX(6px);
  }
}
.animate-shake {
  animation: shake 0.4s ease;
}
</style>
