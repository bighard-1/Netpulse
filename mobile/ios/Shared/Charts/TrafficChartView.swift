import SwiftUI
import Charts

private enum TrafficSeries: String, CaseIterable {
    case inbound = "入方向"
    case outbound = "出方向"
}

private struct TrafficChartPoint: Identifiable {
    let id = UUID()
    let ts: Date
    let series: TrafficSeries
    let value: Double
}

struct TrafficChartView: View {
    let model: TrafficRenderModel
    let showIn: Bool
    let showOut: Bool

    private var allPoints: [TrafficChartPoint] {
        var out: [TrafficChartPoint] = []
        if showIn {
            out.append(contentsOf: model.decimated.compactMap { p in
                guard let v = finite(p.traffic_in_bps) else { return nil }
                return TrafficChartPoint(ts: p.timestamp, series: .inbound, value: v)
            })
        }
        if showOut {
            out.append(contentsOf: model.decimated.compactMap { p in
                guard let v = finite(p.traffic_out_bps) else { return nil }
                return TrafficChartPoint(ts: p.timestamp, series: .outbound, value: v)
            })
        }
        return out.sorted { $0.ts < $1.ts }
    }

    private var yMax: Double {
        max(1.0, model.yMax)
    }

    private var chartWidth: CGFloat {
        let points = max(120, allPoints.count)
        return CGFloat(points) * 1.6
    }

    var body: some View {
        HStack(spacing: 0) {
            VStack(spacing: 0) {
                Text(Fmt.bps(yMax)).font(.caption2).foregroundStyle(.secondary)
                Spacer()
                Text(Fmt.bps(yMax * 0.75)).font(.caption2).foregroundStyle(.secondary)
                Spacer()
                Text(Fmt.bps(yMax * 0.5)).font(.caption2).foregroundStyle(.secondary)
                Spacer()
                Text(Fmt.bps(yMax * 0.25)).font(.caption2).foregroundStyle(.secondary)
                Spacer()
                Text("0 bps").font(.caption2).foregroundStyle(.secondary)
            }
            .frame(width: 74)
            .padding(.vertical, 6)

            ScrollView(.horizontal, showsIndicators: true) {
                Chart(allPoints) { p in
                    LineMark(
                        x: .value("时间", p.ts),
                        y: .value("值", p.value)
                    )
                    .foregroundStyle(by: .value("序列", p.series.rawValue))
                    .lineStyle(by: .value("序列", p.series.rawValue))
                    .interpolationMethod(.linear)
                }
                .chartForegroundStyleScale([
                    TrafficSeries.inbound.rawValue: Color.indigo,
                    TrafficSeries.outbound.rawValue: Color.green
                ])
                // 两条都改为实线，只用颜色区分。
                .chartLineStyleScale([
                    TrafficSeries.inbound.rawValue: StrokeStyle(lineWidth: 2),
                    TrafficSeries.outbound.rawValue: StrokeStyle(lineWidth: 2)
                ])
                .chartYScale(domain: 0...yMax)
                .chartYAxis(.hidden)
                .chartXAxis {
                    AxisMarks(values: .automatic(desiredCount: 6)) { value in
                        AxisGridLine(); AxisTick()
                        AxisValueLabel(format: .dateTime.month().day().hour())
                    }
                }
                .frame(width: chartWidth, height: 300)
            }
        }
    }
}
