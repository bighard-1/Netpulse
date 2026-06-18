export const npChartGrid = { left: "4%", right: "4%", top: 48, bottom: 44, containLabel: true };

export const npChartPalette = {
  inbound: "#6366F1",
  outbound: "#22C55E",
  cpu: "#F97316",
  mem: "#06B6D4",
  storage: "#3B82F6",
  health: "#10B981",
  availability: "#F59E0B",
  danger: "#EF4444",
  warning: "#F59E0B",
  muted: "#94A3B8"
};

export const npAxisLabel = { color: "#64748b", fontSize: 11, fontWeight: 600 };
export const npAxisLine = { lineStyle: { color: "rgba(148,163,184,0.45)" } };
export const npAxisTick = { lineStyle: { color: "rgba(148,163,184,0.35)" } };
export const npSplitLine = { lineStyle: { color: "rgba(148,163,184,0.18)", type: "dashed" } };

export const npLegend = {
  top: 8,
  itemWidth: 10,
  itemHeight: 10,
  icon: "roundRect",
  textStyle: { color: "#475569", fontSize: 12, fontWeight: 700 }
};

export function npEmptyGraphic(text = "当前时间范围暂无数据") {
  return {
    type: "text",
    left: "center",
    top: "middle",
    style: {
      text,
      fill: "#64748b",
      fontSize: 13,
      fontWeight: 700,
      align: "center",
      backgroundColor: "rgba(248,250,252,0.78)",
      borderColor: "rgba(148,163,184,0.28)",
      borderWidth: 1,
      borderRadius: 12,
      padding: [9, 14]
    }
  };
}

export function npTooltip(extra = {}) {
  const { axisPointer, ...rest } = extra;
  const mergedAxisPointer = {
    type: "line",
    animation: false,
    lineStyle: { color: "rgba(99,102,241,0.62)", width: 1, type: "dashed" },
    ...(axisPointer || {}),
    lineStyle: {
      color: "rgba(99,102,241,0.62)",
      width: 1,
      type: "dashed",
      ...(axisPointer?.lineStyle || {})
    }
  };

  return {
    trigger: "axis",
    borderWidth: 1,
    borderColor: "rgba(148,163,184,0.18)",
    backgroundColor: "rgba(15,23,42,0.94)",
    textStyle: { color: "#e2e8f0", fontSize: 12, lineHeight: 20 },
    padding: [10, 12],
    confine: true,
    appendToBody: true,
    axisPointer: mergedAxisPointer,
    extraCssText: "box-shadow:0 18px 42px -18px rgba(15,23,42,.95);border-radius:14px;backdrop-filter:blur(12px);",
    ...rest
  };
}
