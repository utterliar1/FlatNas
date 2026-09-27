// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { mount } from "@vue/test-utils";
import { createAppI18n } from "@/plugins/i18n";
import SimpleWeatherWidget from "./SimpleWeatherWidget.vue";

vi.mock("../stores/main", () => ({
  useMainStore: vi.fn(() => ({ isLogged: true, saveSingleWidget: vi.fn() })),
}));

vi.mock("@/composables/useWeather", () => ({
  useWeather: vi.fn(() => ({
    weather: { value: { temp: "20.0", city: "宁波市", text: "多云", forecast: [] } },
    locationSource: { value: "manual" },
    networkStatus: { value: "online" },
    fetchWeather: vi.fn(),
  })),
}));

const widget = {
  id: "weather-test",
  type: "weather",
  enable: true,
  isPublic: true,
  data: { city: "宁波市" },
};

describe("SimpleWeatherWidget lifecycle", () => {
  afterEach(() => {
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  it("clears the day/night interval on unmount", () => {
    vi.useFakeTimers();
    const clearIntervalSpy = vi.spyOn(globalThis, "clearInterval");

    const wrapper = mount(SimpleWeatherWidget, {
      props: { widget },
      global: { plugins: [createAppI18n()] },
    });

    wrapper.unmount();

    expect(clearIntervalSpy).toHaveBeenCalled();
  });
});
