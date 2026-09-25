<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from "vue";
import { useI18n } from "vue-i18n";
import type { WidgetConfig } from "@/types";
import { useMainStore } from "../stores/main";
import OverlayMotion from "@/components/base/OverlayMotion.vue";

const { t } = useI18n();
const props = defineProps<{ widget: WidgetConfig }>();
const store = useMainStore();

// Initialize data structure
const initData = () => {
  const w = store.widgets.find((item) => item.id === props.widget.id);
  if (w && !w.data) {
    w.data = {
      startTime: new Date().toISOString().slice(0, 16), // Default to now
      title: "正计时", // 数据标识：持久化，禁止 i18n
      style: "card",
      isRunning: false,
      totalPauseDuration: 0,
      pauseStartTime: null,
    };
  } else if (w && w.data) {
    // Ensure new fields exist for migration
    if (w.data.isRunning === undefined) w.data.isRunning = false;
    if (w.data.totalPauseDuration === undefined) w.data.totalPauseDuration = 0;
    if (w.data.pauseStartTime === undefined) w.data.pauseStartTime = null;
    if (w.data.displayFormat === undefined) w.data.displayFormat = "d-hms";
    if (
      w.data.displayFormat === "full-colon" ||
      w.data.displayFormat === "full-slash" ||
      w.data.displayFormat === "full-hyphen"
    ) {
      w.data.displayFormat = "d-hms";
    }
  }
};
initData();

const showConfig = ref(false);
const formData = ref({
  startTime: "",
  title: "",
  style: "card",
  displayFormat: "d-hms",
});

const openConfig = () => {
  const data = props.widget.data || {
    startTime: new Date().toISOString().slice(0, 16),
    title: "正计时", // 数据标识：持久化，禁止 i18n
    style: "card",
    displayFormat: "d-hms",
  };
  formData.value = { ...data };
  showConfig.value = true;
};

const saveConfig = async () => {
  const w = store.widgets.find((item) => item.id === props.widget.id);
  if (w) {
    const timeChanged = w.data?.startTime !== formData.value.startTime;
    w.data = { ...w.data, ...formData.value };
    if (timeChanged) {
      w.data.totalPauseDuration = 0;
      w.data.pauseStartTime = null;
      w.data.isRunning = false;
    }
    store.markDirty();
    calculate();
  }
  showConfig.value = false;
  try {
    await store.saveSingleWidget(props.widget.id, {
      data: { ...w?.data, ...formData.value },
      enable: props.widget.enable,
    });
  } catch {
    // saveSingleWidget 内部已处理错误，此处静默忽略
  }
};

// Timer Logic
const timeDisplay = ref({
  full: t("settings.countupWidget.fmtFullZero"),
  datePart: t("settings.countupWidget.fmtDayZero"),
  timePart: t("settings.countupWidget.fmtHmsZero"),
  days: 0,
  hours: 0,
  minutes: 0,
  seconds: 0,
});
let timer: ReturnType<typeof setInterval> | null = null;

const formatNum = (num: number) => num.toString().padStart(2, "0");
const daysInMonth = (year: number, monthIndex: number) =>
  new Date(year, monthIndex + 1, 0).getDate();
const diffCalendarParts = (startDate: Date, endDate: Date) => {
  let borrow = 0;
  let seconds = endDate.getSeconds() - startDate.getSeconds();
  if (seconds < 0) {
    seconds += 60;
    borrow = 1;
  } else {
    borrow = 0;
  }

  let minutes = endDate.getMinutes() - startDate.getMinutes() - borrow;
  if (minutes < 0) {
    minutes += 60;
    borrow = 1;
  } else {
    borrow = 0;
  }

  let hours = endDate.getHours() - startDate.getHours() - borrow;
  if (hours < 0) {
    hours += 24;
    borrow = 1;
  } else {
    borrow = 0;
  }
  const endDay = endDate.getDate();
  let endMonth = endDate.getMonth();
  let endYear = endDate.getFullYear();

  let days = endDay - startDate.getDate() - borrow;
  if (days < 0) {
    endMonth -= 1;
    if (endMonth < 0) {
      endMonth = 11;
      endYear -= 1;
    }
    days += daysInMonth(endYear, endMonth);
    borrow = 1;
  } else {
    borrow = 0;
  }

  let months = endMonth - startDate.getMonth() - borrow;
  if (months < 0) {
    months += 12;
    borrow = 1;
  } else {
    borrow = 0;
  }

  const years = endYear - startDate.getFullYear() - borrow;

  return { years, months, days, hours, minutes, seconds };
};

const calculate = () => {
  const data = props.widget.data;
  if (!data || !data.startTime) return;

  const now = Date.now();
  let effectiveNow = now;

  if (!data.isRunning && data.pauseStartTime) {
    // If paused, time is frozen at pauseStartTime
    effectiveNow = data.pauseStartTime;
  }

  const start = new Date(data.startTime).getTime();
  // Effective start time shifts by total pause duration
  const effectiveStart = start + (data.totalPauseDuration || 0);

  let diff = effectiveNow - effectiveStart;
  if (diff < 0) diff = 0;

  const totalSeconds = Math.floor(diff / 1000);
  const seconds = totalSeconds % 60;
  const totalMinutes = Math.floor(totalSeconds / 60);
  const minutes = totalMinutes % 60;
  const totalHours = Math.floor(totalMinutes / 60);
  const hours = totalHours % 24;
  const days = Math.floor(totalHours / 24);

  const endDate = new Date(start + diff);
  const calendarParts = diffCalendarParts(new Date(start), endDate);

  const dayStr = days.toString();
  const h = formatNum(hours);
  const m = formatNum(minutes);
  const s = formatNum(seconds);

  let fullStr = "";
  let datePart = "";
  let timePart = "";
  const fmt =
    data.displayFormat === "d-hms" ||
    data.displayFormat === "d-only" ||
    data.displayFormat === "ym-only" ||
    data.displayFormat === "ymd-only" ||
    data.displayFormat === "mdhms" ||
    data.displayFormat === "ymdhms"
      ? data.displayFormat
      : "d-hms";

  if (fmt === "d-only") {
    datePart = t("settings.countupWidget.fmtDay", { n: dayStr });
    timePart = "";
    fullStr = datePart;
  } else if (fmt === "ym-only") {
    datePart = t("settings.countupWidget.fmtYearMonth", {
      y: calendarParts.years,
      m: calendarParts.months,
    });
    timePart = "";
    fullStr = datePart;
  } else if (fmt === "ymd-only") {
    datePart = t("settings.countupWidget.fmtYmd", {
      y: calendarParts.years,
      m: calendarParts.months,
      d: calendarParts.days,
    });
    timePart = "";
    fullStr = datePart;
  } else if (fmt === "mdhms") {
    const totalMonths = calendarParts.years * 12 + calendarParts.months;
    datePart = t("settings.countupWidget.fmtMonthDay", {
      m: totalMonths,
      d: calendarParts.days,
    });
    timePart = t("settings.countupWidget.fmtHms", {
      h: formatNum(calendarParts.hours),
      m: formatNum(calendarParts.minutes),
      s: formatNum(calendarParts.seconds),
    });
    fullStr = `${datePart} ${timePart}`;
  } else if (fmt === "ymdhms") {
    datePart = t("settings.countupWidget.fmtYmd", {
      y: calendarParts.years,
      m: calendarParts.months,
      d: calendarParts.days,
    });
    timePart = t("settings.countupWidget.fmtHms", {
      h: formatNum(calendarParts.hours),
      m: formatNum(calendarParts.minutes),
      s: formatNum(calendarParts.seconds),
    });
    fullStr = `${datePart} ${timePart}`;
  } else {
    datePart = t("settings.countupWidget.fmtDay", { n: dayStr });
    timePart = t("settings.countupWidget.fmtHms", { h, m, s });
    fullStr = `${datePart} ${timePart}`;
  }

  timeDisplay.value = {
    full: fullStr,
    datePart,
    timePart,
    days: fmt === "d-hms" || fmt === "d-only" ? days : calendarParts.days,
    hours: fmt === "d-hms" ? hours : fmt === "mdhms" || fmt === "ymdhms" ? calendarParts.hours : 0,
    minutes:
      fmt === "d-hms" ? minutes : fmt === "mdhms" || fmt === "ymdhms" ? calendarParts.minutes : 0,
    seconds:
      fmt === "d-hms" ? seconds : fmt === "mdhms" || fmt === "ymdhms" ? calendarParts.seconds : 0,
  };
};

const toggleTimer = () => {
  const w = store.widgets.find((item) => item.id === props.widget.id);
  if (!w || !w.data) return;

  if (w.data.isRunning) {
    // Pause
    w.data.isRunning = false;
    w.data.pauseStartTime = Date.now();
  } else {
    // Start/Resume
    const now = Date.now();
    if (w.data.pauseStartTime) {
      w.data.totalPauseDuration = (w.data.totalPauseDuration || 0) + (now - w.data.pauseStartTime);
      w.data.pauseStartTime = null;
    } else {
      // First start? Or just resume without pause time (shouldn't happen if logic is correct)
      // If it was never paused, pauseStartTime is null.
      // If we are starting from fresh (Reset state), totalPauseDuration is 0.
    }
    w.data.isRunning = true;
  }
  store.markDirty();
  calculate(); // Immediate update
};

const resetTimer = () => {
  const w = store.widgets.find((item) => item.id === props.widget.id);
  if (!w || !w.data) return;

  w.data.isRunning = false;
  w.data.totalPauseDuration = 0;
  w.data.pauseStartTime = null;
  store.markDirty();
  calculate();
};

onMounted(() => {
  calculate();
  timer = setInterval(calculate, 1000);
});

onUnmounted(() => {
  if (timer) clearInterval(timer);
});

// Styles
const styles = computed(() => [
  { label: t("settings.countupWidget.styleCard"), value: "card" },
  { label: t("settings.countupWidget.styleSimple"), value: "simple" },
  { label: t("settings.countupWidget.styleNeon"), value: "neon" },
]);

const isSmall = computed(
  () => (props.widget.colSpan ?? 1) <= 1 && (props.widget.rowSpan ?? 1) <= 1,
);
</script>

<template>
  <div
    class="w-full h-full relative group overflow-hidden rounded-2xl transition-all select-none"
    :class="[
      widget.data?.style === 'neon'
        ? 'bg-gray-900 text-white border border-green-500/30'
        : widget.data?.style === 'simple'
          ? 'bg-white/90 text-gray-800 border border-white/40'
          : 'bg-gradient-to-br from-emerald-500 to-teal-600 text-white shadow-lg border border-white/10',
    ]"
  >
    <!-- Config Modal -->
    <OverlayMotion
      :show="showConfig"
      :z-index="50"
      close-on-overlay
      overlay-class="bg-black/60 backdrop-blur-sm p-4"
      panel-class="max-w-sm"
      @close="saveConfig"
    >
        <div class="bg-white text-gray-800 rounded-xl shadow-2xl w-full overflow-hidden flex flex-col">
          <div class="p-4 border-b border-gray-100 flex justify-between items-center bg-gray-50">
            <div class="font-bold text-lg">{{ t('settings.countupWidget.configTitle') }}</div>
            <button @click="saveConfig" class="text-gray-400 hover:text-gray-600">
              <svg
                xmlns="http://www.w3.org/2000/svg"
                class="h-5 w-5"
                viewBox="0 0 20 20"
                fill="currentColor"
              >
                <path
                  fill-rule="evenodd"
                  d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z"
                  clip-rule="evenodd"
                />
              </svg>
            </button>
          </div>
          <div class="p-5 flex flex-col gap-4">
            <div>
              <label class="text-xs font-bold text-gray-500 uppercase tracking-wider block mb-1.5"
                >{{ t('settings.countupWidget.titleLabel') }}</label
              >
              <input
                v-model="formData.title"
                class="w-full px-3 py-2 text-sm border border-gray-200 rounded-lg focus:border-emerald-500 focus:ring-2 focus:ring-emerald-500/20 outline-none transition-all"
                :placeholder="t('settings.countupWidget.titlePlaceholder')"
              />
            </div>
            <div>
              <label class="text-xs font-bold text-gray-500 uppercase tracking-wider block mb-1.5"
                >{{ t('settings.countupWidget.startTime') }}</label
              >
              <input
                type="datetime-local"
                step="1"
                v-model="formData.startTime"
                class="w-full px-3 py-2 text-sm border border-gray-200 rounded-lg focus:border-emerald-500 focus:ring-2 focus:ring-emerald-500/20 outline-none transition-all"
              />
            </div>
            <div>
              <label class="text-xs font-bold text-gray-500 uppercase tracking-wider block mb-1.5"
                >{{ t('settings.countupWidget.formatLabel') }}</label
              >
              <select
                v-model="formData.displayFormat"
                class="w-full px-3 py-2 text-sm border border-gray-200 rounded-lg focus:border-emerald-500 focus:ring-2 focus:ring-emerald-500/20 outline-none transition-all bg-white"
              >
                <option value="d-hms">{{ t('settings.countupWidget.formatDHMS') }}</option>
                <option value="d-only">{{ t('settings.countupWidget.formatDOnly') }}</option>
                <option value="mdhms">{{ t('settings.countupWidget.formatMDHMS') }}</option>
                <option value="ym-only">{{ t('settings.countupWidget.formatYMOnly') }}</option>
                <option value="ymdhms">{{ t('settings.countupWidget.formatYMDHMS') }}</option>
                <option value="ymd-only">{{ t('settings.countupWidget.formatYMDOnly') }}</option>
              </select>
            </div>
            <div>
              <label class="text-xs font-bold text-gray-500 uppercase tracking-wider block mb-1.5"
                >{{ t('settings.countupWidget.styleLabel') }}</label
              >
              <div class="grid grid-cols-3 gap-2">
                <button
                  v-for="s in styles"
                  :key="s.value"
                  @click="formData.style = s.value"
                  class="px-2 py-2 text-xs font-medium rounded-lg border transition-all text-center"
                  :class="
                    formData.style === s.value
                      ? 'border-emerald-500 bg-emerald-50 text-emerald-600'
                      : 'border-gray-200 hover:border-gray-300 text-gray-600'
                  "
                >
                  {{ s.label }}
                </button>
              </div>
            </div>
          </div>
          <div class="p-4 bg-gray-50 border-t border-gray-100">
            <button
              @click="saveConfig"
              :disabled="store.isSaving"
              class="w-full py-2.5 bg-emerald-600 hover:bg-emerald-700 text-white rounded-lg text-sm font-bold transition-colors shadow-sm"
              :class="{ 'opacity-60 cursor-not-allowed': store.isSaving }"
            >
              {{ store.isSaving ? t('settings.countupWidget.saving') : t('settings.countupWidget.saveBtn') }}
            </button>
          </div>
        </div>
    </OverlayMotion>

    <!-- Main Content -->
    <div class="w-full h-full flex flex-col items-center justify-center p-2 relative z-10">
      <!-- Settings Button -->
      <button
        @click.stop="openConfig"
        class="absolute top-2 right-2 opacity-0 group-hover:opacity-100 transition-opacity p-1.5 rounded-full hover:bg-black/10 active:scale-95 z-20"
        :title="t('settings.countupWidget.settings')"
      >
        <svg
          xmlns="http://www.w3.org/2000/svg"
          viewBox="0 0 24 24"
          fill="currentColor"
          class="w-4 h-4"
        >
          <path
            fill-rule="evenodd"
            d="M11.078 2.25c-.917 0-1.699.663-1.85 1.567l-.091.549a.798.798 0 01-.517.608 7.45 7.45 0 00-.478.198.798.798 0 01-.796-.064l-.453-.324a1.875 1.875 0 00-2.416.2l-.043.044a1.875 1.875 0 00-.204 2.416l.325.454a.798.798 0 01.064.796 7.448 7.448 0 00-.198.478.798.798 0 01-.608.517l-.55.092a1.875 1.875 0 00-1.566 1.849v.044c0 .917.663 1.699 1.567 1.85l.549.091c.281.047.508.25.608.517.06.162.127.321.198.478a.798.798 0 01-.064.796l-.324.453a1.875 1.875 0 00.2 2.416l.044.043a1.875 1.875 0 002.416.204l.454-.325a.798.798 0 01.796-.064c.157.071.316.137.478.198.267.1.47.327.517.608l.092.55c.15.903.932 1.566 1.849 1.566h.044c.917 0 1.699-.663 1.85-1.567l.091-.549a.798.798 0 01.517-.608 7.52 7.52 0 00.478-.198.798.798 0 01.796.064l.453.324a1.875 1.875 0 002.416-.2l.043-.044a1.875 1.875 0 00.204-2.416l-.325-.454a.798.798 0 01-.064-.796c.071-.157.137-.316.198-.478.1-.267.327-.47.608-.517l.55-.092a1.875 1.875 0 001.566-1.849v-.044c0-.917-.663-1.699-1.567-1.85l-.549-.091a.798.798 0 01-.608-.517 7.507 7.507 0 00-.198-.478.798.798 0 01.064-.796l.324-.453a1.875 1.875 0 00-.2-2.416l-.044-.043a1.875 1.875 0 00-2.416-.204l-.454.325a.798.798 0 01-.796.064 7.462 7.462 0 00-.478-.198.798.798 0 01-.517-.608l-.092-.55a1.875 1.875 0 00-1.849-1.566h-.044zM12 15.75a3.75 3.75 0 100-7.5 3.75 3.75 0 000 7.5z"
            clip-rule="evenodd"
          />
        </svg>
      </button>

      <!-- Title -->
      <div class="text-xs font-medium opacity-80 mb-1 uppercase tracking-wider truncate max-w-full">
        {{ widget.data?.title || t('settings.countupWidget.title') }}
      </div>

      <!-- Time Display -->
      <div class="flex flex-col items-center">
        <!-- Main Time -->
        <div
          class="font-bold font-mono tracking-tight flex flex-col items-center justify-center"
          :class="[
            isSmall ? 'text-lg' : 'text-2xl',
            widget.data?.style === 'neon'
              ? 'text-transparent bg-clip-text bg-gradient-to-r from-green-300 to-emerald-300 drop-shadow-[0_0_5px_rgba(52,211,153,0.5)]'
              : widget.data?.style === 'simple'
                ? 'text-emerald-600'
                : 'text-white',
          ]"
        >
          <template
            v-if="
              widget.data?.displayFormat === 'd-hms' ||
              widget.data?.displayFormat === 'd-only' ||
              widget.data?.displayFormat === 'mdhms' ||
              widget.data?.displayFormat === 'ym-only' ||
              widget.data?.displayFormat === 'ymdhms' ||
              widget.data?.displayFormat === 'ymd-only'
            "
          >
            <div :class="isSmall ? 'text-2xl' : 'text-4xl opacity-90'">
              {{ timeDisplay.datePart }}
            </div>
            <div v-if="timeDisplay.timePart">{{ timeDisplay.timePart }}</div>
          </template>
          <template v-else>
            {{ timeDisplay.full }}
          </template>
        </div>
      </div>

      <!-- Controls -->
      <div class="flex items-center gap-2 mt-2" v-if="!isSmall">
        <button
          @click.stop="toggleTimer"
          class="p-1.5 rounded-full hover:bg-black/20 transition-colors active:scale-95"
          :title="widget.data?.isRunning ? t('settings.countupWidget.pause') : t('settings.countupWidget.start')"
        >
          <svg
            v-if="!widget.data?.isRunning"
            xmlns="http://www.w3.org/2000/svg"
            class="h-5 w-5"
            viewBox="0 0 20 20"
            fill="currentColor"
          >
            <path
              fill-rule="evenodd"
              d="M10 18a8 8 0 100-16 8 8 0 000 16zM9.555 7.168A1 1 0 008 8v4a1 1 0 001.555.832l3-2a1 1 0 000-1.664l-3-2z"
              clip-rule="evenodd"
            />
          </svg>
          <svg
            v-else
            xmlns="http://www.w3.org/2000/svg"
            class="h-5 w-5"
            viewBox="0 0 20 20"
            fill="currentColor"
          >
            <path
              fill-rule="evenodd"
              d="M18 10a8 8 0 11-16 0 8 8 0 0116 0zM7 8a1 1 0 012 0v4a1 1 0 11-2 0V8zm5-1a1 1 0 00-1 1v4a1 1 0 102 0V8a1 1 0 00-1-1z"
              clip-rule="evenodd"
            />
          </svg>
        </button>
        <button
          @click.stop="resetTimer"
          class="p-1.5 rounded-full hover:bg-black/20 transition-colors active:scale-95"
          :title="t('settings.countupWidget.reset')"
        >
          <svg
            xmlns="http://www.w3.org/2000/svg"
            class="h-4 w-4"
            viewBox="0 0 20 20"
            fill="currentColor"
          >
            <path
              fill-rule="evenodd"
              d="M4 2a1 1 0 011 1v2.101a7.002 7.002 0 0111.601 2.566 1 1 0 11-1.885.666A5.002 5.002 0 005.999 7H9a1 1 0 010 2H4a1 1 0 01-1-1V3a1 1 0 011-1zm.008 9.057a1 1 0 011.276.61A5.002 5.002 0 0014.001 13H11a1 1 0 110-2h5a1 1 0 011 1v5a1 1 0 11-2 0v-2.101a7.002 7.002 0 01-11.601-2.566 1 1 0 01.61-1.276z"
              clip-rule="evenodd"
            />
          </svg>
        </button>
      </div>
    </div>
  </div>
</template>
