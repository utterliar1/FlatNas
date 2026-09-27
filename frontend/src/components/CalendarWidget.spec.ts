// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { mount } from "@vue/test-utils";
import { createTestingPinia } from "@pinia/testing";
import { createAppI18n } from "@/plugins/i18n";
import CalendarWidget from "./CalendarWidget.vue";

const widget = {
  id: "calendar-test",
  type: "calendar",
  enable: true,
  isPublic: true,
  data: { style: "day" },
};

describe("CalendarWidget lifecycle", () => {
  afterEach(() => {
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  it("does not start the minute interval after unmounting before alignment", () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-09-27T01:32:30.000+08:00"));
    const setIntervalSpy = vi.spyOn(globalThis, "setInterval");

    const wrapper = mount(CalendarWidget, {
      props: { widget },
      global: {
        plugins: [createAppI18n(), createTestingPinia({ createSpy: vi.fn })],
      },
    });

    wrapper.unmount();
    vi.advanceTimersByTime(90_000);

    expect(setIntervalSpy).not.toHaveBeenCalled();
  });
});
