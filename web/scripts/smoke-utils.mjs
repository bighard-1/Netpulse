import assert from "node:assert/strict";

Object.defineProperty(globalThis, "localStorage", {
  value: {
    getItem: () => null,
    setItem: () => {},
    removeItem: () => {}
  },
  configurable: true
});

const {
  formatApiErrorMessage,
  getApiError,
  getApiErrorDetail,
  isAuthExpiredError,
  isForbiddenError
} = await import("../src/utils/apiError.js");
const {
  filterDashboardDevices,
  findDashboardPorts,
  rankPortsBySpeed
} = await import("../src/utils/dashboard.js");
const {
  calcTrafficFetchPlan,
  compactValidPoints,
  decimatePoints,
  fillShortNullGaps,
  pickTrafficUnit,
  roundUpNice,
  toTrafficSeriesData,
  xAxisLabelFormatter
} = await import("../src/utils/portTraffic.js");

function testApiError() {
  const err = {
    code: "ECONNABORTED",
    response: {
      status: 500,
      data: {
        code: "ERR_INTERNAL",
        error: "服务异常",
        hint: "稍后重试"
      }
    }
  };
  const detail = getApiErrorDetail(err, "默认错误");
  assert.equal(getApiError(err, "默认错误"), "服务异常");
  assert.equal(detail.code, "ERR_INTERNAL");
  assert.equal(detail.status, 500);
  assert.equal(detail.isTimeout, true);
  assert.equal(formatApiErrorMessage(detail), "服务异常 | 错误码: ERR_INTERNAL | 提示: 稍后重试");
  assert.equal(isAuthExpiredError({ response: { status: 401, data: { error: "invalid token" } } }), true);
  assert.equal(isForbiddenError({ response: { status: 403, data: { error: "admin only" } } }), true);
}

function testDashboardUtils() {
  const devices = [
    {
      id: 1,
      name: "Core-A",
      ip: "10.0.0.1",
      status: "online",
      interfaces: [
        { id: 11, index: 1, name: "XGE1/0/1", remark: "上联", speed_mbps: 10000, traffic_in_bps: 300, traffic_out_bps: 500 },
        { id: 12, index: 2, name: "GE1/0/2", remark: "办公", speed_mbps: 1000, traffic_in_bps: 100, traffic_out_bps: 100 }
      ]
    },
    {
      id: 2,
      name: "Access-B",
      ip: "10.0.0.2",
      status: "offline",
      interfaces: [{ id: 21, index: 1, name: "Ethernet1", remark: "大厅", speed_mbps: 100, traffic_in_bps: 50, traffic_out_bps: 10 }]
    }
  ];
  assert.equal(filterDashboardDevices(devices, "core", "all").length, 1);
  assert.equal(filterDashboardDevices(devices, "", "offline").length, 1);
  assert.equal(findDashboardPorts(devices, "上联")[0].portId, 11);
  assert.equal(rankPortsBySpeed(devices, 1000, 10000)[0].interfaceId, 12);
  assert.equal(rankPortsBySpeed(devices, 10000, 0)[0].bps, 800);
}

function testPortTrafficUtils() {
  assert.deepEqual(pickTrafficUnit(1_200_000), { unit: "Mbps", div: 1_000_000 });
  assert.equal(roundUpNice(355_200_000), 500_000_000);
  const series = toTrafficSeriesData([
    { timestamp: "2026-06-01T00:00:00Z", traffic_in_bps: 100, traffic_out_bps: 200 },
    { timestamp: "bad", traffic_in_bps: 1, traffic_out_bps: 2 },
    { timestamp: "2026-06-01T00:01:00Z", traffic_in_bps: null, traffic_out_bps: "300" },
    { timestamp: "2026-06-01T00:02:00Z", traffic_in_bps: null, traffic_in_status: "PORT_DOWN", traffic_out_bps: null, traffic_out_status: "PORT_DOWN" }
  ]);
  assert.equal(series.inbound.length, 2);
  assert.equal(series.inbound[1][1], null);
  assert.equal(series.outbound.length, 3);
  assert.equal(series.outbound[1][1], 300);
  assert.equal(series.outbound[2][1], null);

  const filled = fillShortNullGaps([[1, 10], [2, null], [3, 30]], 1);
  assert.equal(filled[1][1], 20);
  assert.equal(compactValidPoints(filled).length, 3);
  assert.equal(decimatePoints(Array.from({ length: 10 }, (_, i) => [i, i]), 4).at(-1)[0], 9);

  const oneDay = calcTrafficFetchPlan(new Date("2026-06-01T00:00:00Z"), new Date("2026-06-01T12:00:00Z"), 10000, 30);
  assert.equal(oneDay.interval, "2m");
  const sevenDays = calcTrafficFetchPlan(new Date("2026-06-01T00:00:00Z"), new Date("2026-06-07T00:00:00Z"), 1000, 60);
  assert.equal(sevenDays.interval, "15m");
  const longerThanSevenDays = calcTrafficFetchPlan(new Date("2026-06-01T00:00:00Z"), new Date("2026-06-08T00:00:01Z"), 1000, 60);
  assert.equal(longerThanSevenDays.interval, "1h");
  assert.equal(xAxisLabelFormatter(new Date("2026-06-10T00:00:00+08:00").getTime(), "d30"), "10日");
}

testApiError();
testDashboardUtils();
testPortTrafficUtils();

console.log("web utility smoke tests passed");
