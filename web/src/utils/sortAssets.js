const tierRank = {
  core: 0,
  核心: 0,
  aggregation: 1,
  aggregate: 1,
  agg: 1,
  汇聚: 1,
  access: 2,
  接入: 2
};

function normalizeTier(device) {
  const raw = String(device?.device_tier || device?.tier || "").trim().toLowerCase();
  if (raw.includes("核心")) return "core";
  if (raw.includes("汇聚")) return "aggregation";
  if (raw.includes("接入")) return "access";
  return raw || "access";
}

export function sortAssets(list) {
  return [...(list || [])].sort((a, b) => {
    const ar = tierRank[normalizeTier(a)] ?? 99;
    const br = tierRank[normalizeTier(b)] ?? 99;
    if (ar !== br) return ar - br;
    const an = String(a?.name || a?.remark || a?.ip || "");
    const bn = String(b?.name || b?.remark || b?.ip || "");
    return an.localeCompare(bn, "zh-Hans-CN-u-co-pinyin", {
      numeric: true,
      sensitivity: "base"
    });
  });
}
