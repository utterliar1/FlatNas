import { describe, it, expect, vi } from 'vitest';
import { createI18n } from 'vue-i18n';

/**
 * 词条「消息编译」回归测试。
 *
 * 背景（2026-09-26 真实线上报障）：
 *   `settings.extraSections.customCssInputPlaceholder` 等 3 条词条是 **代码示例**，
 *   里面带 CSS/JS 的花括号。vue-i18n 会把 `{` 当成插值占位符 → 消息编译报错：
 *     - 开发环境：`Message compilation error: Unterminated closing brace`（回落成原文，只是日志难看）
 *     - **生产构建**：抛 `SyntaxError: 7`（生产环境错误文案只剩数字码）→ 渲染该面板时抛异常，
 *       整个设置弹窗被销毁 —— 表现为主观上的「设置 - 开放中心这个入口点不进去」。
 *   修复方式：代码示例里的花括号必须写成 vue-i18n 的字面量语法 `{'{'}` / `{'}'}`。
 *
 * 本测试的价值：词条由人（含翻译）手改，很容易再引入裸花括号。
 * 它把「所有语言、所有词条都必须能被编译器接受」钉成断言，在 CI 上拦下来。
 * 注意：**不能用 `expect(() => t(key)).not.toThrow()` 来断言** —— 开发环境下 vue-i18n
 * 只打 console.error 并回落到原文，并不会抛错（抛错只发生在生产构建），那样断言会假绿。
 * 所以这里改为「编译错误日志嗅探」。
 */

type Json = Record<string, unknown>;

// 用 glob 静态收集，新增语言/新命名空间会自动纳入，不需要维护清单
const modules = import.meta.glob<{ default: Json }>('../locales/*/*.json', { eager: true });

const byLocale: Record<string, Record<string, Json>> = {};
for (const [path, mod] of Object.entries(modules)) {
  const m = /\.\.\/locales\/([^/]+)\/([^/]+)\.json$/.exec(path);
  if (!m) continue;
  const locale = m[1] as string;
  const ns = m[2] as string;
  byLocale[locale] = byLocale[locale] ?? {};
  byLocale[locale]![ns] = mod.default;
}

/** 展平出所有叶子词条的路径（相对命名空间） */
function leafPaths(node: Json, prefix = ''): string[] {
  const out: string[] = [];
  for (const [k, v] of Object.entries(node)) {
    const p = prefix ? `${prefix}.${k}` : k;
    if (v && typeof v === 'object') out.push(...leafPaths(v as Json, p));
    else if (typeof v === 'string') out.push(p);
  }
  return out;
}

const COMPILE_ERROR_MARK = 'Message compilation error';

describe('i18n 词条消息编译', () => {
  it('全部语言 / 全部词条都能被 vue-i18n 编译通过', () => {
    const locales = Object.keys(byLocale).sort();
    expect(locales.length).toBeGreaterThan(0);

    const spy = vi.spyOn(console, 'error').mockImplementation(() => {});
    const failures: string[] = [];
    let checked = 0;

    try {
      for (const locale of locales) {
        const nsMap = byLocale[locale]!;
        const i18n = createI18n({
          legacy: false,
          locale,
          fallbackLocale: locale,
          messages: { [locale]: nsMap },
        });
        for (const [ns, tree] of Object.entries(nsMap)) {
          for (const p of leafPaths(tree)) {
            checked += 1;
            spy.mockClear();
            i18n.global.t(`${ns}.${p}`);
            const bad = spy.mock.calls
              .map((c) => String(c[0]))
              .filter((s) => s.includes(COMPILE_ERROR_MARK));
            if (bad.length) {
              // 只取第一行，避免把带 ^ 的代码片段整段塞进报错信息
              failures.push(`${locale} :: ${ns}.${p} -> ${bad[0]!.split('\n')[0]}`);
            }
          }
        }
      }
    } finally {
      spy.mockRestore();
    }

    expect(checked).toBeGreaterThan(1000); // 防止 glob 失效导致空跑假绿
    expect(
      failures,
      `以下词条无法被 vue-i18n 编译（代码示例里的花括号请写成 {'{'} / {'}'}）：\n${failures.join('\n')}`,
    ).toEqual([]);
  });

  it('代码示例词条渲染后是原样的花括号（转义正确性，而非仅仅“能编译”）', () => {
    // 这三条都是「代码示例」占位符，里面必须有真实的花括号
    const codeSampleKeys = [
      'settings.extraSections.customCssInputPlaceholder',
      'settings.extraSections.customJsInputPlaceholder',
      'settings.customCss.jsPlaceholder',
    ];
    for (const locale of Object.keys(byLocale).sort()) {
      const i18n = createI18n({
        legacy: false,
        locale,
        fallbackLocale: locale,
        messages: { [locale]: byLocale[locale]! },
      });
      for (const key of codeSampleKeys) {
        const rendered = i18n.global.t(key);
        // 渲染结果里不该残留 vue-i18n 的字面量语法
        expect(rendered, `${locale} :: ${key}`).not.toContain("{'{'}");
        expect(rendered, `${locale} :: ${key}`).not.toContain("{'}'}");
        // 花括号必须成对出现（转义漏了一半就会出现不平衡）
        const opens = (rendered.match(/\{/g) ?? []).length;
        const closes = (rendered.match(/\}/g) ?? []).length;
        expect(opens, `${locale} :: ${key} 渲染后花括号不成对`).toBeGreaterThan(0);
        expect(closes, `${locale} :: ${key} 渲染后花括号不成对`).toBe(opens);
      }
    }
  });
});
