<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useMainStore } from "../stores/main";
import OverlayMotion from "@/components/base/OverlayMotion.vue";

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

watch(
  () => props.show,
  (v) => {
    if (v) {
      unlocked.value = sessionStorage.getItem(UNLOCK_KEY) === "1";
      code.value = "";
      confirmCode.value = "";
      error.value = "";
      shake.value = false;
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
    showError("请输入访问码");
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
      showError(data.error || "访问码不正确");
    }
  } catch {
    showError("网络错误，请重试");
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

const handleSetup = async () => {
  if (busy.value) return;
  const c = code.value.trim();
  if (!c) {
    showError("请输入访问码");
    return;
  }
  if (c.length < 4) {
    showError("访问码至少 4 位");
    return;
  }
  if (c !== confirmCode.value.trim()) {
    showError("两次输入不一致");
    return;
  }
  busy.value = true;
  try {
    const ok = await store.updateSystemConfig({ accessCode: c });
    if (ok) {
      emit("changed");
      close();
    } else {
      showError("设置失败（需要管理员身份）");
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
          {{ unlocked ? "已解锁" : setupMode ? "设置访问码" : "访问验证" }}
        </div>
      </div>

      <!-- Body -->
      <div class="px-5 pb-5 pt-3 space-y-3">
        <!-- 已解锁：提供重新上锁 -->
        <template v-if="unlocked">
          <p class="text-[11px] text-gray-400 text-center leading-relaxed">
            隐藏分组当前可见。重新上锁后需再次输入访问码。
          </p>
          <button
            @click="handleLock"
            :disabled="busy"
            class="w-full py-2 rounded-xl text-sm font-bold text-white bg-gray-800 hover:bg-black transition-colors disabled:opacity-50"
          >
            重新上锁
          </button>
        </template>

        <!-- 管理员首次设置 -->
        <template v-else-if="setupMode">
          <p class="text-[11px] text-gray-400 text-center leading-relaxed">
            设置全局访问码后，标记为「访问码保护」的分组将被隐藏，输入访问码才能显示。
          </p>
          <input
            v-model="code"
            type="password"
            autocomplete="off"
            placeholder="访问码（至少 4 位）"
            class="w-full px-3 py-2 border border-gray-200 rounded-xl text-sm outline-none focus:border-gray-800 text-center tracking-widest"
            @keyup.enter="handleSetup"
          />
          <input
            v-model="confirmCode"
            type="password"
            autocomplete="off"
            placeholder="再次输入以确认"
            class="w-full px-3 py-2 border border-gray-200 rounded-xl text-sm outline-none focus:border-gray-800 text-center tracking-widest"
            @keyup.enter="handleSetup"
          />
          <button
            @click="handleSetup"
            :disabled="busy"
            class="w-full py-2 rounded-xl text-sm font-bold text-white bg-gray-800 hover:bg-black transition-colors disabled:opacity-50"
          >
            保存访问码
          </button>
        </template>

        <!-- 常规输码解锁 -->
        <template v-else>
          <input
            v-model="code"
            type="password"
            autocomplete="off"
            placeholder="访问码"
            class="w-full px-3 py-2 border border-gray-200 rounded-xl text-sm outline-none focus:border-gray-800 text-center tracking-widest"
            @keyup.enter="handleUnlock"
          />
          <button
            @click="handleUnlock"
            :disabled="busy"
            class="w-full py-2 rounded-xl text-sm font-bold text-white bg-gray-800 hover:bg-black transition-colors disabled:opacity-50"
          >
            确认
          </button>
        </template>

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
