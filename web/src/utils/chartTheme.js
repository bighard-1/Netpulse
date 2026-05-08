export const npChartGrid = { left: "3%", right: "4%", bottom: "10%", containLabel: true };

export const npAxisLabel = { color: "#64748b", fontSize: 11 };
export const npAxisLine = { lineStyle: { color: "#cbd5e1" } };
export const npSplitLine = { lineStyle: { color: "rgba(148,163,184,0.2)" } };

export function npTooltip(extra = {}) {
  return {
    trigger: "axis",
    borderWidth: 0,
    backgroundColor: "rgba(15,23,42,0.92)",
    textStyle: { color: "#e2e8f0", fontSize: 12 },
    padding: [8, 10],
    extraCssText: "box-shadow:0 12px 28px -16px rgba(15,23,42,.9);border-radius:10px;",
    ...extra
  };
}

