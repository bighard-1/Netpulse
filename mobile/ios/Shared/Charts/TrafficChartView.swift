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

    @State private var zoomScale: CGFloat = 1.0
    @State private var baseZoom: CGFloat = 1.0

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
        return CGFloat(points) * 1.6 * zoomScale
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

            ScrollViewReader { proxy in
                ScrollView(.horizontal, showsIndicators: true) {
                    HStack(spacing: 0) {
                        chartCore
                            .frame(width: chartWidth, height: 300)
                        Color.clear
                            .frame(width: 1, height: 1)
                            .id("end-anchor")
                    }
                }
                .onAppear {
                    proxy.scrollTo("end-anchor", anchor: .trailing)
                }
                .onChange(of: chartWidth) { _ in
                    proxy.scrollTo("end-anchor", anchor: .trailing)
                }
            }
        }
    }

    private var chartCore: some View {
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
        .chartLineStyleScale([
            TrafficSeries.inbound.rawValue: StrokeStyle(lineWidth: 2),
            TrafficSeries.outbound.rawValue: StrokeStyle(lineWidth: 2)
        ])
        .chartYScale(domain: 0...yMax)
        .chartYAxis(.hidden)
        .chartXAxis {
            AxisMarks(values: .automatic(desiredCount: 6)) { value in
                AxisGridLine(); AxisTick()
                AxisValueLabel(format: .dateTime.hour().minute())
            }
        }
        .gesture(
            MagnificationGesture()
                .onChanged { scale in
                    let next = baseZoom * scale
                    zoomScale = min(6.0, max(1.0, next))
                }
                .onEnded { _ in
                    baseZoom = zoomScale
                }
        )
    }
}

// 用于导出图片，避免 ScrollView 导致空白画布。
struct TrafficChartExportView: View {
    let model: TrafficRenderModel
    let showIn: Bool
    let showOut: Bool

    var body: some View {
        HStack(spacing: 0) {
            VStack(spacing: 0) {
                Text(Fmt.bps(max(1.0, model.yMax))).font(.caption2).foregroundStyle(.secondary)
                Spacer(); Text(Fmt.bps(model.yMax * 0.75)).font(.caption2).foregroundStyle(.secondary)
                Spacer(); Text(Fmt.bps(model.yMax * 0.5)).font(.caption2).foregroundStyle(.secondary)
                Spacer(); Text(Fmt.bps(model.yMax * 0.25)).font(.caption2).foregroundStyle(.secondary)
                Spacer(); Text("0 bps").font(.caption2).foregroundStyle(.secondary)
            }
            .frame(width: 74)

            VStack(alignment: .leading, spacing: 8) {
                HStack(spacing: 16) {
                    Label("入方向", systemImage: "line.diagonal")
                        .foregroundStyle(Color(red: 0.22, green: 0.36, blue: 0.95))
                    Label("出方向", systemImage: "line.diagonal")
                        .foregroundStyle(Color(red: 0.04, green: 0.73, blue: 0.43))
                }
                .font(.caption)

                Chart {
                    if showIn {
                        ForEach(model.decimated.compactMap { p -> TrafficChartPoint? in
                            guard let v = finite(p.traffic_in_bps) else { return nil }
                            return TrafficChartPoint(ts: p.timestamp, series: .inbound, value: v)
                        }) { p in
                            LineMark(x: .value("时间", p.ts), y: .value("值", p.value))
                                .lineStyle(StrokeStyle(lineWidth: 2.6))
                                .foregroundStyle(Color(red: 0.22, green: 0.36, blue: 0.95))
                        }
                    }
                    if showOut {
                        ForEach(model.decimated.compactMap { p -> TrafficChartPoint? in
                            guard let v = finite(p.traffic_out_bps) else { return nil }
                            return TrafficChartPoint(ts: p.timestamp, series: .outbound, value: v)
                        }) { p in
                            LineMark(x: .value("时间", p.ts), y: .value("值", p.value))
                                .lineStyle(StrokeStyle(lineWidth: 2.6))
                                .foregroundStyle(Color(red: 0.04, green: 0.73, blue: 0.43))
                        }
                    }
                }
                .chartYScale(domain: 0...max(1.0, model.yMax))
                .chartYAxis(.hidden)
                .chartXAxis {
                    AxisMarks(values: .automatic(desiredCount: 8)) { value in
                        AxisGridLine(); AxisTick()
                        AxisValueLabel(format: .dateTime.hour().minute())
                    }
                }
                .frame(height: 520)
            }
        }
    }
}
