import { readonly, ref } from "vue";

// 管理界面解锁状态：默认锁定（纯展示模式）。
// 在设定时间窗口内连续点击指定次数（如右上角"在线"徽标）切换解锁/锁定。
const unlocked = ref(false);
const CLICKS_TO_TOGGLE = 3;
const CLICK_WINDOW_MS = 2000;

let clickCount = 0;
let lastClickAt = 0;
let resetTimer: ReturnType<typeof setTimeout> | undefined;

export function useAdminUnlock() {
  const registerSecretClick = () => {
    const now = Date.now();
    if (now - lastClickAt > CLICK_WINDOW_MS) clickCount = 0;
    lastClickAt = now;
    clickCount += 1;
    if (resetTimer) clearTimeout(resetTimer);
    resetTimer = setTimeout(() => {
      clickCount = 0;
    }, CLICK_WINDOW_MS);
    if (clickCount >= CLICKS_TO_TOGGLE) {
      clickCount = 0;
      unlocked.value = !unlocked.value;
    }
  };

  return { unlocked: readonly(unlocked), registerSecretClick };
}
