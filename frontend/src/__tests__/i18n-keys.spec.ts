/**
 * i18n 词条路径存在性回归测试
 *
 * 背景：本项目的消息树是 `{ common: common.json, settings: settings.json }`，
 * 而 common.json 内部又有一层 `common` 子对象 —— 因此通用词条的真实路径是
 * `common.common.xxx` 而不是 `common.xxx`，极易写错；此外组件里手写的 key
 * 一旦与实际语言包脱节，运行时只会静默显示 key 字面量，不会报错。
 *
 * 本测试扫描 src 下所有静态 `t("…")` 调用，逐一校验其在 zh-CN 语言包中存在，
 * 把「词条名写错 / 漏翻 / 命名空间前缀写错」挡在测试阶段。
 *
 * 只校验**静态字符串** key；含模板变量或字符串拼接的动态 key 会被自动跳过
 * （无法静态判定的部分由 TypeScript 与人工审查兜底）。
 */
import { describe, it, expect } from "vitest";
import * as fs from "node:fs";
import * as path from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const SRC_DIR = path.resolve(__dirname, "..");
const LOCALES_DIR = path.join(SRC_DIR, "locales");

/** 与 plugins/i18n.ts 的 createAppI18n 保持一致的 messages 结构 */
const messages: Record<string, unknown> = {
  common: JSON.parse(fs.readFileSync(path.join(LOCALES_DIR, "zh-CN", "common.json"), "utf-8")),
  settings: JSON.parse(fs.readFileSync(path.join(LOCALES_DIR, "zh-CN", "settings.json"), "utf-8")),
};

function lookup(dotPath: string): boolean {
  let cur: unknown = messages;
  for (const seg of dotPath.split(".")) {
    if (cur && typeof cur === "object" && seg in (cur as Record<string, unknown>)) {
      cur = (cur as Record<string, unknown>)[seg];
    } else {
      return false;
    }
  }
  return true;
}

/** 收集 src 下的源码文件（排除测试自身） */
function collectFiles(dir: string, out: string[] = []): string[] {
  for (const e of fs.readdirSync(dir, { withFileTypes: true })) {
    const p = path.join(dir, e.name);
    if (e.isDirectory()) {
      if (e.name === "node_modules" || e.name === "__tests__") continue;
      collectFiles(p, out);
    } else if (/\.(vue|ts)$/.test(e.name)) {
      out.push(p);
    }
  }
  return out;
}

// lookbehind 排除标识符前缀（避免误报 format( / obj.t(），matchAll 避免正则状态残留
const CALL_RE = /(?<![A-Za-z0-9_$])\$?t\s*\(\s*(["'`])([A-Za-z][A-Za-z0-9_.-]*)\1/g;

describe("i18n 词条路径存在性", () => {
  const files = collectFiles(SRC_DIR);
  const invalid: { file: string; line: number; key: string }[] = [];
  let checked = 0;

  for (const f of files) {
    const src = fs.readFileSync(f, "utf-8");
    for (const m of src.matchAll(CALL_RE)) {
      const key = m[2];
      if (!key.includes(".")) continue;
      checked++;
      if (!lookup(key)) {
        invalid.push({
          file: path.relative(SRC_DIR, f).replace(/\\/g, "/"),
          line: src.slice(0, m.index).split("\n").length,
          key,
        });
      }
    }
  }

  it("源码中应存在静态 t() 调用（防止扫描逻辑失效导致空跑）", () => {
    expect(checked).toBeGreaterThan(200);
  });

  it("所有静态 t() 路径都应在 zh-CN 语言包中真实存在", () => {
    const report = invalid.map((x) => `  ${x.file}:${x.line} -> ${x.key}`).join("\n");
    expect(invalid, `以下词条路径不存在，运行时将显示 key 字面量：\n${report}`).toEqual([]);
  });

  it("通用词条必须使用 common.common.* 双重前缀（messages 结构决定）", () => {
    // 防回归：common.json 内有 common 子对象，漏掉一层会取到 undefined
    expect(lookup("common.common.confirm")).toBe(true);
    expect(lookup("common.confirm")).toBe(false);
    expect(lookup("common.auth.login")).toBe(true);
  });
});

// ─────────────────────────────────────────────────────────────
// 已接线组件：模板区不得残留硬编码中文
//
// 每完成一批 i18n 接线，就把组件名加进下面的白名单。
// 这样任何"改回硬编码中文"的改动都会立刻被拦下，且不会误伤尚未接线的组件。
// （只检查 <template> 区并剔除 HTML 注释；<script> 区的注释、开发者日志、
//   以及作为数据标识的中文常量不在检查范围内。）
// ─────────────────────────────────────────────────────────────
const WIRED_COMPONENTS = [
  // 批次 1：登录 / 认证 / 解锁入口
  "LoginModal.vue",
  "PasswordConfirmModal.vue",
  "AccessCodeModal.vue",
  // 批次 2：导航侧栏 / 状态指示
  "AppSidebar.vue",
  "GroupSelector.vue",
  "ProxyToggle.vue",
  "SizeSelector.vue",
  "NetworkIndicator.vue",
  "IconShape.vue",
  // 批次 3：主面板 / 弹窗 / 自定义组件 / 正计时
  "GridPanel.vue",
  "EditModal.vue",
  "GroupSettingsModal.vue",
  "CustomCssWidget.vue",
  "CountUpWidget.vue",
  // 批次 4（i18n 收尾扫描）：剩余全部组件
  "SettingsModal.vue",
  "WallpaperLibrary.vue",
  "MusicWidget.vue",
  "MiniPlayer.vue",
  "FileTransferWidget.vue",
  "MemoWidget.vue",
  "RssSettings.vue",
  "RssWidget.vue",
  "SearchSettings.vue",
  "ScriptManager.vue",
  "AmapWeatherWidget.vue",
  "SimpleWeatherWidget.vue",
  "ClockWeatherWidget.vue",
  "ClockWidget.vue",
  "StatusMonitor.vue",
  "Memo/MemoToolbar.vue",
  "IconSelectionModal.vue",
  "IconUploader.vue",
  "TodoWidget.vue",
  "CountdownWidget.vue",
  "CalendarWidget.vue",
  "HotWidget.vue",
  "BookmarkWidget.vue",
  "IframeWidget.vue",
];

describe("已接线组件的模板不应残留硬编码中文", () => {
  for (const name of WIRED_COMPONENTS) {
    it(`${name} 的 <template> 中不应有中文字面量`, () => {
      const file = path.join(SRC_DIR, "components", name);
      expect(fs.existsSync(file), `${name} 不存在`).toBe(true);
      const src = fs.readFileSync(file, "utf-8");

      const m = src.match(/<template>([\s\S]*)<\/template>/);
      expect(m, `${name} 应包含 <template> 区块`).toBeTruthy();

      // 剔除 HTML 注释后再检查
      const body = m![1].replace(/<!--[\s\S]*?-->/g, "");
      // 报错时给出**文件绝对行号**（此前用 template 内相对行号，容易误导排查）
      const baseLine = src.slice(0, src.indexOf("<template>") + "<template>".length).split("\n").length - 1;
      const hits = body
        .split("\n")
        .map((line, i) => ({ line: line.trim(), no: baseLine + i + 1 }))
        .filter((x) => /[\u4e00-\u9fa5]/.test(x.line));

      const report = hits.map((h) => `  L${h.no}: ${h.line}`).join("\n");
      expect(hits, `${name} 模板仍有硬编码中文，请改用 t()：\n${report}`).toEqual([]);
    });
  }
});
