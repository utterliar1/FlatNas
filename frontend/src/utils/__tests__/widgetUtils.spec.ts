/**
 * 回归：normalizeIncomingWidgets 必须保证 widget id 全局唯一。
 *
 * 背景（2026-09-26 定位到的真实缺陷）：
 *   当用户数据里的组件「占用了默认组件的 id、但 type 不同」时（例如历史残留把
 *   `w1` 分配给了 music），补齐默认组件的逻辑只按 `type` 判重，于是会把默认列表里
 *   id 同样为 `w1` 的 clock 直接 push 进去 —— 产生两个 id 相同的组件。
 *
 *   网格库 grid-layout-plus 在挂载时会校验 `Layout[i].i` 唯一，重复即 **throw**
 *   （`VueGridLayout: Layout[n].i must be unique!`），并中断整棵网格的挂载：
 *   首页组件区整个渲染不出来。这正对应隔离实例里长期存在的
 *   `Layout[3].i must be unique!` 页面级告警。
 *
 * 因此这里锁死两条不变量：
 *   1. normalizeIncomingWidgets 的输出 id 必须唯一；
 *   2. 经 generateLayout 产出的 `i` 也必须唯一（即网格层不会再撞）。
 */
import { describe, it, expect } from "vitest";
import { normalizeIncomingWidgets } from "@/utils/widgetUtils";
import { generateLayout } from "@/utils/gridLayout";
import type { WidgetConfig } from "@/types";

/** 复刻 GridPanel 中参与网格渲染的类型白名单。 */
const GRID_WIDGET_TYPES = new Set([
  "clock", "weather", "calendar", "memo", "todo", "music", "calculator", "ip",
  "div-card", "countdown", "countup", "iframe", "bookmarks", "hot",
  "clockweather", "amap-weather", "rss", "custom-css", "file-transfer",
]);

const duplicates = (xs: string[]) => [...new Set(xs.filter((x, i) => xs.indexOf(x) !== i))];

/** 构造测试用组件，补齐 WidgetConfig 的必填字段。 */
const w = (id: string, type: string, extra: Partial<WidgetConfig> = {}): WidgetConfig => ({
  id,
  type,
  enable: true,
  isPublic: false,
  ...extra,
});

/** 走一遍 GridPanel 的真实链路：可见性过滤 → 排序 → generateLayout。 */
function gridLayoutIds(widgets: WidgetConfig[], colNum = 4) {
  const visible = widgets
    .filter((w) => w.enable !== false && GRID_WIDGET_TYPES.has(w.type))
    .sort((a, b) => (a.y ?? 0) - (b.y ?? 0) || (a.x ?? 0) - (b.x ?? 0));
  return generateLayout(visible, colNum).map((l) => l.i);
}

describe("normalizeIncomingWidgets — widget id 唯一性", () => {
  it("复现用例：数据里 w1/w2 被 search/music 占用时，补齐默认组件不得产生重复 id", () => {
    // 这正是设置弹窗探针里用的数据形态，曾稳定复现 Layout[3].i must be unique!
    const input: WidgetConfig[] = [
      w("w1", "search"),
      w("w2", "music"),
    ];

    const out = normalizeIncomingWidgets(input, true);
    const ids = out.map((w) => w.id);

    expect(duplicates(ids)).toEqual([]);
    // 原始两项必须原样保留（不能因为去重把用户数据改掉）
    expect(out.some((w) => w.id === "w1" && w.type === "search")).toBe(true);
    expect(out.some((w) => w.id === "w2" && w.type === "music")).toBe(true);
    // 缺失的默认组件仍应补齐（clock 等 type 不在数据里）
    expect(out.some((w) => w.type === "clock")).toBe(true);

    // 端到端：网格层也不能再出现重复的 i
    expect(duplicates(gridLayoutIds(out))).toEqual([]);
  });

  it("任意「id 与默认项冲突」的组合都能被修复", () => {
    const defaults = normalizeIncomingWidgets([], true);
    // 逐个把默认项的 id 抢走给一个不同 type 的组件
    for (const target of defaults) {
      const squatter = w(target.id, target.type === "music" ? "bookmarks" : "music");
      const out = normalizeIncomingWidgets([squatter], true);
      const ids = out.map((w) => w.id);
      expect(duplicates(ids), `抢走 ${target.id} 后出现重复`).toEqual([]);
      expect(duplicates(gridLayoutIds(out)), `抢走 ${target.id} 后网格 i 重复`).toEqual([]);
    }
  });

  it("用户数据自身就带重复 id 时也要收敛为唯一", () => {
    const input: WidgetConfig[] = [
      w("dup", "memo"),
      w("dup", "todo"),
      w("dup", "calculator"),
    ];
    const out = normalizeIncomingWidgets(input, true);
    expect(duplicates(out.map((w) => w.id))).toEqual([]);
    // 至少保留第一个 dup 原 id
    expect(out.filter((w) => w.id === "dup")).toHaveLength(1);
    // 三份用户数据都要保留（登录态还会补齐默认组件，故只断言 >= 3）
    const userTypes = out.filter((w) => ["memo", "todo", "calculator"].includes(w.type));
    expect(userTypes.length).toBeGreaterThanOrEqual(3); // 不吞组件
  });

  it("访客模式（不补默认项）同样保持唯一", () => {
    const input: WidgetConfig[] = [
      w("w1", "search", { isPublic: true }),
      w("w1", "memo", { isPublic: true }),
    ];
    const out = normalizeIncomingWidgets(input, false);
    expect(duplicates(out.map((w) => w.id))).toEqual([]);
  });

  it("正常数据（无冲突）不应被改写", () => {
    const input: WidgetConfig[] = [
      { id: "w1", type: "clock", enable: true, x: 0, y: 0, w: 1, h: 1, isPublic: true },
      { id: "w2", type: "weather", enable: true, x: 1, y: 0, w: 1, h: 1, isPublic: true },
    ];
    const out = normalizeIncomingWidgets(input, true);
    expect(out.find((w) => w.type === "clock")!.id).toBe("w1");
    expect(out.find((w) => w.type === "weather")!.id).toBe("w2");
  });

  it("空数据时使用默认列表，id 本身唯一", () => {
    for (const logged of [true, false]) {
      const out = normalizeIncomingWidgets([], logged);
      expect(duplicates(out.map((w) => w.id))).toEqual([]);
      expect(duplicates(gridLayoutIds(out))).toEqual([]);
    }
  });

  it("真实仓库数据（server/data/users/admin.json）经链路后无重复 i", async () => {
    const fs = await import("node:fs");
    const path = await import("node:path");
    const file = path.resolve(process.cwd(), "..", "server", "data", "users", "admin.json");
    if (!fs.existsSync(file)) return; // 精简检出时跳过
    const data = JSON.parse(fs.readFileSync(file, "utf-8")) as { widgets: WidgetConfig[] };
    const out = normalizeIncomingWidgets(data.widgets, true);
    expect(duplicates(out.map((w) => w.id))).toEqual([]);
    expect(duplicates(gridLayoutIds(out))).toEqual([]);
  });
});
