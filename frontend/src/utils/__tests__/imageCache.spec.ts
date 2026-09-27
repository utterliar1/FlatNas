import { describe, it, expect, beforeEach } from "vitest";
import { getCachedImage } from "../imageCache";

// 回归：cache_meta_* 存的是 JSON，一旦被手动改动 / 写入中断 / 跨版本残留导致
// 内容损坏，getCachedImage 在最早期就会 JSON.parse。修复前这里直接抛异常，
// 会把调用方（壁纸加载链）打断；修复后应清理脏键并按「无缓存」返回 null。
describe("imageCache.getCachedImage 脏数据兜底", () => {
  const url = "https://example.com/a.jpg";
  const key = `cache_meta_${url}`;

  beforeEach(() => {
    localStorage.clear();
  });

  it("meta 损坏（非法 JSON）时不抛异常，清理脏键并返回 null", async () => {
    localStorage.setItem(key, "{not valid json");
    await expect(getCachedImage(url)).resolves.toBeNull();
    expect(localStorage.getItem(key)).toBeNull();
  });

  it("meta 为合法 JSON 但结构异常（timestamp 非数字）时按过期处理", async () => {
    localStorage.setItem(key, JSON.stringify({ timestamp: "oops" }));
    // timestamp 非法 → 归一化为 0 → 视为过期 → 返回 null（不抛异常）
    await expect(getCachedImage(url)).resolves.toBeNull();
  });

  it("无 meta 时返回 null", async () => {
    await expect(getCachedImage(url)).resolves.toBeNull();
  });
});
