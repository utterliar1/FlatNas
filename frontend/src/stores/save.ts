import { ref, watch } from "vue";
import { defineStore } from "pinia";
import pako from "pako";
import * as offlineQueue from "@/utils/offlineQueue";
import {
  stripWidgetUiState,
  stripForceNetworkMode,
  normalizeVersion,
  buildServerLayoutMap,
  buildServerLayoutSignature,
} from "@/utils/storeHelpers";
import { useAuthStore } from "./auth";
import { useWidgetsStore } from "./widgets";
import { useGroupsStore } from "./groups";
import { useConfigStore } from "./config";
import { useCacheStore } from "./cache";
import { useNetworkStore } from "./network";

export const useSaveStore = defineStore("save", () => {
  const auth = useAuthStore();
  const widgetsStore = useWidgetsStore();
  const groupsStore = useGroupsStore();
  const configStore = useConfigStore();
  const cacheStore = useCacheStore();
  const networkStore = useNetworkStore();

  let saveTimer: ReturnType<typeof setTimeout> | null = null;
  const isSaving = ref(false);
  const hasPendingSave = ref(false);
  let lastSavedJson = "";
  const hasUnsavedChanges = ref(false);

  // 三方合并基线：最近一次「与服务端一致」的合并字段快照（不含 version/password）。
  // 随每次保存/服务端数据应用而重置。后端以此为 base 做 base/local/server 逐字段合并，
  // 从而在多端同时编辑时只让「真正改动的字段」生效，杜绝整份文档互相覆盖。
  const baseJson = ref("");

  // 合并过程中的字段级冲突提示（非阻塞）：多端改同一字段时后端以服务端为准并回报。
  const mergeNotice = ref("");
  let mergeNoticeTimer: ReturnType<typeof setTimeout> | null = null;
  // 合并结果引入了本端之外的变化 → 需回拉服务端以收敛视图（由 sync 包装层消费）。
  const resyncRequested = ref(false);

  const conflictState = ref({ show: false, serverVersion: 0, clientVersion: 0 });
  const conflictResolving = ref(false);

  const offlineQueueCount = ref(0);
  const offlineQueueConflictState = ref<{
    show: boolean;
    item: { baseVersion: number; data: Record<string, unknown> } | null;
    serverVersion: number;
  }>({ show: false, item: null, serverVersion: 0 });

  const syncConfirmModal = ref({ show: false, serverVersion: 0 });
  let heartbeatLostSinceLastVisible = false;

  const markDirty = () => {
    if (auth.isLogged) hasUnsavedChanges.value = true;
  };

  const saveCustomScripts = async () => {
    try {
      if (!auth.isLogged) return;
      const res = await fetch("/api/custom-scripts", { method: "POST", headers: cacheStore.getHeaders(), body: JSON.stringify({ css: configStore.appConfig.customCssList || [], js: configStore.appConfig.customJsList || [] }) });
      if (!res.ok) console.error("Failed to save custom scripts");
    } catch (e) { console.error("Error saving custom scripts", e); }
  };

  const jsonEqual = (left: unknown, right: unknown) =>
    JSON.stringify(left ?? null) === JSON.stringify(right ?? null);

  // 排序键的 JSON.stringify，避免键序不同导致伪冲突
  const stableStringify = (obj: unknown): string => {
    if (obj === null || obj === undefined) return "null";
    if (typeof obj !== "object") return JSON.stringify(obj);
    if (Array.isArray(obj)) return "[" + obj.map(stableStringify).join(",") + "]";
    const sorted = Object.keys(obj as Record<string, unknown>).sort();
    return "{" + sorted.map((k) => JSON.stringify(k) + ":" + stableStringify((obj as Record<string, unknown>)[k])).join(",") + "}";
  };

  // 构造参与合并的「干净」字段集（不含 version/password），既作为 base 基线，
  // 也用于保存载荷。与后端 mergeableKeys 保持一致。
  const buildCleanBody = (feeds: unknown[], cats: unknown[]): Record<string, unknown> => ({
    groups: groupsStore.groups,
    groupOrder: Array.isArray(groupsStore.groupOrder) ? groupsStore.groupOrder : [],
    widgets: widgetsStore.widgets.map((w) => stripWidgetUiState(w)),
    appConfig: stripForceNetworkMode(configStore.appConfig as unknown as Record<string, unknown>),
    rssFeeds: feeds,
    rssCategories: cats,
  });

  // 以当前内存状态（= 刚与服务端同步后的状态）重置合并基线。
  const seedBase = (feeds: unknown[], cats: unknown[]) => {
    baseJson.value = JSON.stringify(buildCleanBody(feeds, cats));
  };

  const setMergeNotice = (msg: string) => {
    if (!msg) return;
    mergeNotice.value = msg;
    if (mergeNoticeTimer) clearTimeout(mergeNoticeTimer);
    mergeNoticeTimer = setTimeout(() => { mergeNotice.value = ""; mergeNoticeTimer = null; }, 8000);
  };

  const consumeResyncRequest = (): boolean => {
    if (!resyncRequested.value) return false;
    resyncRequested.value = false;
    return true;
  };

  const saveData = async (
    immediate = false,
    force = false,
    dataVersion: { value: number },
    rssFeeds: { value: unknown[] },
    rssCategories: { value: unknown[] },
    fetchData: () => Promise<void>,
  ): Promise<"saved" | "no_change" | "conflict" | "unauthorized" | "queued"> => {
    if (saveTimer) { clearTimeout(saveTimer); saveTimer = null; }
    if (conflictResolving.value && !force) { hasPendingSave.value = true; return "no_change"; }

    const doSave = async () => {
      if (conflictState.value.show && !force) { hasPendingSave.value = false; return "conflict"; }
      if (configStore.isPageUnloading) return "no_change";
      if (cacheStore.isCacheWriteGuardActive()) { cacheStore.deferredSaveRequested = true; return "no_change"; }
      if (isSaving.value) { hasPendingSave.value = true; return "no_change"; }

      isSaving.value = true;
      hasPendingSave.value = false;

      try {
        if (!auth.isLogged) return "unauthorized";
        if (force && conflictState.value.show) {
          dataVersion.value = normalizeVersion(conflictState.value.serverVersion);
        }

        const body: Record<string, unknown> = {
          ...buildCleanBody(rssFeeds.value, rssCategories.value),
          version: dataVersion.value,
        };
        if (typeof auth.password === "string" && auth.password.length > 0) {
          body.password = auth.password;
        }
        const json = JSON.stringify(body);
        if (json === lastSavedJson) {
          // 内容与上次保存完全一致：没有真正未保存的变更，清掉脏标记
          // （否则自动保存调度器会因脏标记滞留而反复空转）
          hasUnsavedChanges.value = false;
          return "no_change";
        }

        cacheStore.saveToCache(body);
        // 带 base 基线走「字段级三方合并」；无基线（首次/旧缓存）时退化为后端旧的全量+版本校验逻辑。
        const wireBody: Record<string, unknown> = { ...body };
        if (baseJson.value) {
          try {
            wireBody.base = JSON.parse(baseJson.value);
          } catch { /* 基线损坏则忽略，走旧逻辑 */ }
        }
        const compressed = pako.gzip(JSON.stringify(wireBody));

        const getSaveTimeout = () => {
          if (configStore.effectiveIsLan) return 15000;
          if (configStore.forceNetworkMode === "latency") return 120000;
          return 60000;
        };

        const MAX_SAVE_RETRIES = 3;
        const SAVE_TIMEOUT_MS = getSaveTimeout();
        let saveAttempt = 0;
        let res: Response | null = null;

        while (saveAttempt < MAX_SAVE_RETRIES) {
          saveAttempt++;
          try {
            const controller = new AbortController();
            const timeout = window.setTimeout(() => controller.abort(), SAVE_TIMEOUT_MS);
            res = await fetch("/api/save", { method: "POST", headers: { ...cacheStore.getHeaders(), "Content-Encoding": "gzip" }, body: compressed, signal: controller.signal }).finally(() => window.clearTimeout(timeout));
            if (res.ok || res.status === 409 || res.status === 401) break;
            if (saveAttempt < MAX_SAVE_RETRIES) {
              const delay = Math.min(1000 * Math.pow(2, saveAttempt - 1), 5000);
              await new Promise((r) => setTimeout(r, delay));
            }
          } catch (e) {
            if (e instanceof DOMException && e.name === "AbortError" && saveAttempt < MAX_SAVE_RETRIES) {
              const delay = Math.min(1000 * Math.pow(2, saveAttempt - 1), 5000);
              await new Promise((r) => setTimeout(r, delay));
              continue;
            }
            throw e;
          }
        }

        if (!res) throw new Error(`Save failed after ${MAX_SAVE_RETRIES} retries`);

        if (res.ok) {
          conflictState.value.show = false;
          hasUnsavedChanges.value = false;
          const result = await res.json().catch(() => null);
          if (result && typeof (result as { version?: number }).version !== "undefined") {
            dataVersion.value = normalizeVersion((result as { version?: number }).version);
          }
          lastSavedJson = JSON.stringify({ ...body, version: dataVersion.value });
          // 保存成功后，本端与服务端在「本端改动字段」上已一致，重置基线为刚提交的干净字段集。
          baseJson.value = JSON.stringify(buildCleanBody(rssFeeds.value, rssCategories.value));
          // 三方合并结果处理（仅合并模式返回这些字段）
          const conflicts = (result as { conflicts?: Array<{ scope?: string; field?: string; id?: string }> } | null)?.conflicts;
          if (Array.isArray(conflicts) && conflicts.length > 0) {
            console.warn("[Merge] 检测到并发修改冲突，已按服务端为准合并：", conflicts);
            setMergeNotice(`检测到 ${conflicts.length} 处多端并发修改冲突，已按服务端为准自动合并。`);
          }
          if ((result as { resync?: boolean } | null)?.resync === true) {
            resyncRequested.value = true;
          }
          widgetsStore.updateLastSavedLayout();
          if (body.password) auth.password = "";
          saveCustomScripts();
          return "saved";
        }

        if (res.status === 409) {
          const result = await res.json().catch(() => null);
          const serverVer = (result as { currentVersion?: number } | null)?.currentVersion;
          if (typeof serverVer !== "undefined") {
            const v = normalizeVersion(serverVer);
            if (conflictState.value.show) return "conflict";
            // Smart conflict check: skip popup if only widget data changed
            try {
              const rd = await (await fetch("/api/data", { headers: cacheStore.getHeaders() })).json();
              const rSig = buildServerLayoutSignature(buildServerLayoutMap(rd.widgets || []));
              const lSig = buildServerLayoutSignature(buildServerLayoutMap(widgetsStore.widgets));
              const rCfg = stripForceNetworkMode((rd.appConfig || {}) as Record<string, unknown>);
              const lCfg = stripForceNetworkMode(configStore.appConfig as unknown as Record<string, unknown>);
              const rssFeedsMatch = jsonEqual(rd.rssFeeds || [], rssFeeds.value);
              const rssCategoriesMatch = jsonEqual(rd.rssCategories || [], rssCategories.value);
              if (
                rSig === lSig &&
                jsonEqual(rd.groups || [], groupsStore.groups) &&
                jsonEqual(rCfg, lCfg) &&
                rssFeedsMatch &&
                rssCategoriesMatch
              ) {
                dataVersion.value = v; await fetchData(); widgetsStore.updateLastSavedLayout(); return "saved";
              }
            } catch (e) { console.warn("Smart conflict check failed", e); }
            // Per-widget LWW 合并：差异仅在个别 widget data 时静默合并
            try {
              const rd = await (await fetch("/api/data", { headers: cacheStore.getHeaders() })).json();
              const serverWidgets = (rd.widgets || []) as any[];
              const localWidgets = widgetsStore.widgets as any[];
              const serverMap = new Map(serverWidgets.map((w: any) => [w.id, w]));
              const localMap = new Map(localWidgets.map((w: any) => [w.id, w]));

              let canAutoMerge = true;
              const mergedWidgets = [...localWidgets];

              for (const [id, sw] of serverMap) {
                const lw = localMap.get(id);
                if (!lw) {
                  // 服务端新增的 widget，直接采用
                  mergedWidgets.push(sw);
                } else if (stableStringify(sw.data) !== stableStringify(lw.data)) {
                  // data 不同 → 检查本端是否修改过
                  let localModified = false;
                  try {
                    const lastSaved = JSON.parse(lastSavedJson || "{}") as { widgets?: any[] };
                    const lastSavedW = (lastSaved.widgets || []).find((w: any) => w.id === id);
                    if (lastSavedW && stableStringify(lastSavedW.data) !== stableStringify(lw.data)) {
                      localModified = true;
                    }
                  } catch { /* lastSavedJson 解析失败视为本端未修改 */ }
                  if (localModified) {
                    canAutoMerge = false;
                    break;
                  }
                  // 本端未修改，采用服务端
                  const idx = mergedWidgets.findIndex((w: any) => w.id === id);
                  if (idx >= 0) mergedWidgets[idx] = sw;
                }
              }

              // 检查服务端删除的 widget（本地有但服务端没有）
              if (canAutoMerge) {
                for (const [id, lw] of localMap) {
                  if (!serverMap.has(id)) {
                    // 检查本端是否修改过该 widget
                    let localModified = false;
                    try {
                      const lastSaved = JSON.parse(lastSavedJson || "{}") as { widgets?: any[] };
                      const lastSavedW = (lastSaved.widgets || []).find((w: any) => w.id === id);
                      if (!lastSavedW || stableStringify(lastSavedW.data) !== stableStringify(lw.data)) {
                        localModified = true;
                      }
                    } catch { localModified = true; }
                    if (localModified) {
                      canAutoMerge = false;
                      break;
                    }
                    // 本端未修改，服务端已删除 → 从合并结果中移除
                    const idx = mergedWidgets.findIndex((w: any) => w.id === id);
                    if (idx >= 0) mergedWidgets.splice(idx, 1);
                  }
                }
              }

              if (canAutoMerge) {
                // 自动合并成功，用合并后的数据重新保存
                const bodyAppConfig =
                  body.appConfig && typeof body.appConfig === "object"
                    ? (body.appConfig as Record<string, unknown>)
                    : {};
                const mergedBody: Record<string, unknown> = {
                  ...body,
                  widgets: mergedWidgets.map((w: any) => stripWidgetUiState(w)),
                  groups: rd.groups || body.groups,
                  version: v,
                };
                if (rd.appConfig)
                  mergedBody.appConfig = {
                    ...bodyAppConfig,
                    ...(rd.appConfig as Record<string, unknown>),
                  };
                const mr = await fetch("/api/save", { method: "POST", headers: cacheStore.getHeaders(), body: JSON.stringify(mergedBody) });
                if (mr.ok) {
                  conflictState.value.show = false; hasUnsavedChanges.value = false;
                  const mrd = await mr.json().catch(() => null);
                  dataVersion.value = mrd && typeof (mrd as { version?: number }).version !== "undefined" ? normalizeVersion((mrd as { version?: number }).version) : v + 1;
                  lastSavedJson = JSON.stringify({ ...mergedBody, version: dataVersion.value });
                  widgetsStore.updateLastSavedLayout();
                  if (body.password) auth.password = "";
                  return "saved";
                }
              }
            } catch (e) { console.warn("LWW merge failed", e); }
            // Retry with adopted version
            const rb = { ...body, version: v };
            const retryController = new AbortController();
            const retryTimeout = setTimeout(() => retryController.abort(), 60000);
            const rr = await fetch("/api/save", { method: "POST", headers: cacheStore.getHeaders(), body: JSON.stringify(rb), signal: retryController.signal }).finally(() => clearTimeout(retryTimeout));
            if (rr.ok) {
              conflictState.value.show = false; hasUnsavedChanges.value = false;
              const rrd = await rr.json().catch(() => null);
              dataVersion.value = rrd && typeof (rrd as { version?: number }).version !== "undefined" ? normalizeVersion((rrd as { version?: number }).version) : v + 1;
              lastSavedJson = JSON.stringify({ ...rb, version: dataVersion.value });
              widgetsStore.updateLastSavedLayout();
              if (body.password) auth.password = "";
              return "saved";
            }
            // Show popup only if structure changed
            const cur = buildServerLayoutSignature(buildServerLayoutMap(widgetsStore.widgets));
            const lChg = cur !== widgetsStore.lastSavedLayoutSignature;
            let gUnch = false;
            let rssFeedsUnch = false;
            let rssCategoriesUnch = false;
            try {
              const lb = JSON.parse(lastSavedJson) as {
                groups?: unknown;
                rssFeeds?: unknown;
                rssCategories?: unknown;
              } | null;
              if (lb) {
                gUnch = jsonEqual(groupsStore.groups, lb.groups || []);
                rssFeedsUnch = jsonEqual(rssFeeds.value, lb.rssFeeds || []);
                rssCategoriesUnch = jsonEqual(rssCategories.value, lb.rssCategories || []);
              }
            } catch { }
            if (!lChg && gUnch && rssFeedsUnch && rssCategoriesUnch) {
              dataVersion.value = v; await fetchData(); hasPendingSave.value = false; return "saved";
            }
            conflictState.value = { show: true, serverVersion: v, clientVersion: dataVersion.value };
            hasPendingSave.value = false;
          }
          return "conflict";
        }

        if (res.status === 401) {
          auth.token = ""; auth.username = "";
          localStorage.removeItem("flat-nas-token"); localStorage.removeItem("flat-nas-username");
          return "unauthorized";
        }

        throw new Error("Save failed");
      } catch (e) {
        if (configStore.isPageUnloading) return "no_change";
        console.error("Save failed, enqueueing to offline queue", e);
        try {
          const fallbackBody: Record<string, unknown> = {
            groups: groupsStore.groups,
            // 与正常保存路径保持一致：断网降级保存也必须带上 groupOrder，
            // 否则离线期间的排序偏好会在队列回放时丢失。
            groupOrder: Array.isArray(groupsStore.groupOrder) ? groupsStore.groupOrder : [],
            widgets: widgetsStore.widgets.map((w) => stripWidgetUiState(w)),
            appConfig: stripForceNetworkMode(configStore.appConfig as unknown as Record<string, unknown>),
            rssFeeds: rssFeeds.value,
            rssCategories: rssCategories.value,
            version: dataVersion.value,
          };
          // 离线队列回放时同样携带基线，让合并策略在恢复联网后依然生效。
          if (baseJson.value) {
            try {
              fallbackBody.base = JSON.parse(baseJson.value);
            } catch { /* ignore */ }
          }
          await offlineQueue.enqueue(fallbackBody, dataVersion.value);
          offlineQueueCount.value = await offlineQueue.size();
          hasPendingSave.value = true;
          return "queued";
        } catch (queueErr) {
          console.error("Failed to enqueue to offline queue", queueErr);
        }
        throw e;
      } finally {
        isSaving.value = false;
        if (hasPendingSave.value) doSave();
      }
    };

    if (immediate) return doSave();
    return new Promise((resolve, reject) => {
      saveTimer = setTimeout(() => {
        saveTimer = null;
        doSave().then(resolve).catch(reject);
      }, 500);
    });
  };

  const resolveConflict = async (
    action: "remote" | "local",
    fetchData: () => Promise<void>,
    saveDataFn: (immediate: boolean, force: boolean) => Promise<string>,
  ) => {
    conflictState.value.show = false;
    conflictResolving.value = true;
    try {
      if (action === "remote") await fetchData();
      else await saveDataFn(true, true);
    } finally {
      conflictResolving.value = false;
    }
  };

  const checkVersionAfterActivation = async (
    isLogged: boolean,
    dataVersion: number,
    fetchVersionOnly: () => Promise<number>,
  ) => {
    if (!isLogged) return;
    heartbeatLostSinceLastVisible = false;
    try {
      const serverVer = await fetchVersionOnly();
      if (serverVer > dataVersion) {
        syncConfirmModal.value = { show: true, serverVersion: serverVer };
      }
    } catch { /* ignore */ }
  };

  const confirmSyncFromServer = async (fetchData: () => Promise<void>) => {
    syncConfirmModal.value = { show: false, serverVersion: 0 };
    await fetchData();
  };

  const dismissSyncConfirm = () => {
    syncConfirmModal.value = { show: false, serverVersion: 0 };
  };

  const resolveOfflineQueueConflict = async (
    action: "force_save" | "discard",
    fetchData: () => Promise<void>,
  ) => {
    if (action === "discard") {
      await offlineQueue.clear();
      offlineQueueCount.value = 0;
      offlineQueueConflictState.value = { show: false, item: null, serverVersion: 0 };
      await fetchData();
      return;
    }
    const items = await offlineQueue.getAll();
    if (items.length === 0) { offlineQueueConflictState.value.show = false; return; }
    const latestItem = items[items.length - 1];
    await offlineQueue.clear();
    offlineQueueConflictState.value.show = false;
    const body = latestItem.data as Record<string, unknown>;
    const json = JSON.stringify(body);
    const compressed = pako.gzip(json);
    try {
      const controller = new AbortController();
      const timeout = setTimeout(() => controller.abort(), 120000);
      const res = await fetch("/api/save", {
        method: "POST",
        headers: { ...cacheStore.getHeaders(), "Content-Encoding": "gzip" },
        body: compressed,
        signal: controller.signal,
      }).finally(() => clearTimeout(timeout));
      if (res.ok) {
        const result = await res.json().catch(() => null);
        if (result && typeof (result as { version?: number }).version !== "undefined") {
          // dataVersion updated by caller
        }
        hasUnsavedChanges.value = false;
      }
    } catch (e) {
      console.error("[OfflineQueue] Force save failed:", e);
    }
  };

  const triggerOfflineQueueReplay = async (
    fetchVersionOnly: () => Promise<number>,
    dataVersion: { value: number },
    getHeaders: () => Record<string, string>,
  ) => {
    const qSize = await offlineQueue.size();
    if (qSize === 0) return;
    console.log(`[OfflineQueue] Starting replay of ${qSize} items`);
    await offlineQueue.replay(
      fetchVersionOnly,
      async (data) => {
        try {
          const compressed = pako.gzip(JSON.stringify(data));
          const c = new AbortController();
          const t = setTimeout(() => c.abort(), 5000);
          const res = await fetch("/api/save", { method: "POST", headers: { ...getHeaders(), "Content-Encoding": "gzip" }, body: compressed, signal: c.signal }).finally(() => clearTimeout(t));
          if (res.ok) {
            const r = await res.json().catch(() => null);
            if (r && typeof (r as { version?: number }).version !== "undefined") dataVersion.value = normalizeVersion((r as { version?: number }).version);
            return true;
          }
          return false;
        } catch { return false; }
      },
      async (widgetId, data, widgetVersion) => {
        try {
          const body = { ...data, version: dataVersion.value, widgetVersion };
          const c = new AbortController();
          const t = setTimeout(() => c.abort(), 5000);
          const res = await fetch(`/api/widgets/${encodeURIComponent(widgetId)}`, { method: "PUT", headers: { ...getHeaders(), "Content-Type": "application/json" }, body: JSON.stringify(body), signal: c.signal }).finally(() => clearTimeout(t));
          if (res.ok) {
            const r = await res.json().catch(() => null);
            if (r && typeof (r as { version?: number }).version !== "undefined") dataVersion.value = normalizeVersion((r as { version?: number }).version);
            return true;
          }
          return false;
        } catch { return false; }
      },
      (pendingItem, serverVersion) => {
        offlineQueueConflictState.value = { show: true, item: pendingItem, serverVersion };
      },
      (item, error) => {
        console.error(`[OfflineQueue] Non-recoverable error for ${item.id}:`, error);
        offlineQueueConflictState.value = { show: true, item, serverVersion: 0 };
      },
    );
    offlineQueueCount.value = await offlineQueue.size();
  };

  const discardOfflineQueue = async (fetchData: () => Promise<void>) => {
    await offlineQueue.clear();
    offlineQueueCount.value = 0;
    offlineQueueConflictState.value = { show: false, item: null, serverVersion: 0 };
    await fetchData();
  };

  return {
    saveTimer,
    isSaving,
    hasPendingSave,
    hasUnsavedChanges,
    conflictState,
    conflictResolving,
    offlineQueueCount,
    offlineQueueConflictState,
    syncConfirmModal,
    heartbeatLostSinceLastVisible,
    markDirty,
    saveData,
    baseJson,
    mergeNotice,
    seedBase,
    consumeResyncRequest,
    resolveConflict,
    checkVersionAfterActivation,
    confirmSyncFromServer,
    dismissSyncConfirm,
    resolveOfflineQueueConflict,
    triggerOfflineQueueReplay,
    discardOfflineQueue,
    saveCustomScripts,
  };
});
