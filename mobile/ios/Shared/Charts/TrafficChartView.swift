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
            out.append(contentsOf: inboundPoints)
        }
        if showOut {
            out.append(contentsOf: outboundPoints)
        }
        return out.sorted { $0.ts < $1.ts }
    }

    private var inboundPoints: [TrafficChartPoint] {
        model.decimated.compactMap { p in
            guard let v = finite(p.traffic_in_bps) else { return nil }
            return TrafficChartPoint(ts: p.timestamp, series: .inbound, value: v)
        }
    }

    private var outboundPoints: [TrafficChartPoint] {
        model.decimated.compactMap { p in
            guard let v = finite(p.traffic_out_bps) else { return nil }
            return TrafficChartPoint(ts: p.timestamp, series: .outbound, value: v)
        }
    }

    private var yStep: Double {
        if model.yMax >= 1_000_000 {
            let maxMbps = model.yMax / 1_000_000
            let stepMbps = max(10.0, ceil(maxMbps / 4.0 / 10.0) * 10.0)
            return stepMbps * 1_000_000
        }
        let maxKbps = model.yMax / 1_000
        let stepKbps = max(10.0, ceil(maxKbps / 4.0 / 10.0) * 10.0)
        return stepKbps * 1_000
    }

    private var yAxisMax: Double {
        yStep * 4.0
    }

    private var yTicks: [Double] {
        [0, 1, 2, 3, 4].map { Double($0) * yStep }
    }

    private var chartWidth: CGFloat {
        let points = max(120, allPoints.count)
        return CGFloat(points) * 1.6 * zoomScale
    }

    var body: some View {
        HStack(spacing: 0) {
            VStack(spacing: 0) {
                Text(yTickLabel(yAxisMax)).font(.caption2).foregroundStyle(.secondary)
                Spacer()
                Text(yTickLabel(yStep * 3)).font(.caption2).foregroundStyle(.secondary)
                Spacer()
                Text(yTickLabel(yStep * 2)).font(.caption2).foregroundStyle(.secondary)
                Spacer()
                Text(yTickLabel(yStep)).font(.caption2).foregroundStyle(.secondary)
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
        Chart {
            ForEach(yTicks, id: \.self) { tick in
                RuleMark(y: .value("YTick", tick))
                    .foregroundStyle(Color.white.opacity(0.10))
                    .lineStyle(StrokeStyle(lineWidth: 0.8))
            }

            ForEach(allPoints) { p in
                LineMark(
                    x: .value("时间", p.ts),
                    y: .value("值", p.value),
                    series: .value("线段", p.series.rawValue)
                )
                .foregroundStyle(by: .value("序列", p.series.rawValue))
                .lineStyle(by: .value("序列", p.series.rawValue))
                .interpolationMethod(.linear)
            }
        }
        .chartForegroundStyleScale([
            TrafficSeries.inbound.rawValue: Color.indigo,
            TrafficSeries.outbound.rawValue: Color.green
        ])
        .chartLineStyleScale([
            TrafficSeries.inbound.rawValue: StrokeStyle(lineWidth: 2),
            TrafficSeries.outbound.rawValue: StrokeStyle(lineWidth: 2)
        ])
        .chartYScale(domain: 0...yAxisMax)
        .chartYAxis(.hidden)
        .chartXAxis {
            AxisMarks(values: xAxisValues(zoomScale: zoomScale)) { value in
                AxisGridLine(); AxisTick()
                AxisValueLabel {
                    if let date = value.as(Date.self) {
                        Text(xAxisLabel(date))
                    }
                }
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

    private func xAxisLabel(_ date: Date) -> String {
        let cal = Calendar.current
        let hm = cal.dateComponents([.hour, .minute], from: date)
        let h = hm.hour ?? 0
        let m = hm.minute ?? 0
        if h == 0 && m == 0 { return date.formatted(.dateTime.month().day()) }
        return date.formatted(.dateTime.hour().minute())
    }

    private func yTickLabel(_ value: Double) -> String {
        if yStep >= 1_000_000 {
            return "\(Int((value / 1_000_000).rounded())) Mbps"
        }
        return "\(Int((value / 1_000).rounded())) Kbps"
    }

    private func xAxisValues(zoomScale: CGFloat) -> [Date] {
        let times = allPoints.map(\.ts)
        guard let start = times.min(), let end = times.max(), start < end else { return [] }
        let span = end.timeIntervalSince(start)
        let visibleSpan = Int(span / max(1.0, Double(zoomScale)))
        let minuteStep: Int
        switch visibleSpan {
        case ..<10800: minuteStep = 15
        case ..<21600: minuteStep = 30
        case ..<86400: minuteStep = 60
        case ..<259200: minuteStep = 180
        case ..<1209600: minuteStep = 360
        default: minuteStep = 720
        }
        return buildTimeAxisValues(start: start, end: end, minuteStep: minuteStep)
    }
}

// 用于导出图片，避免 ScrollView 导致空白画布。
struct TrafficChartExportView: View {
    let model: TrafficRenderModel
    let showIn: Bool
    let showOut: Bool

    private struct ExportPoint: Identifiable {
        let id = UUID()
        let ts: Date
        let value: Double
        let seriesName: String
        let segmentName: String
    }

    private var exportInPoints: [ExportPoint] {
        guard showIn else { return [] }
        return inboundSegments.enumerated().flatMap { pair in
            let (idx, segment) = pair
            return segment.map { p in
                ExportPoint(
                    ts: p.ts,
                    value: p.value,
                    seriesName: "入方向",
                    segmentName: "in-\(idx)"
                )
            }
        }
    }

    private var exportOutPoints: [ExportPoint] {
        guard showOut else { return [] }
        return outboundSegments.enumerated().flatMap { pair in
            let (idx, segment) = pair
            return segment.map { p in
                ExportPoint(
                    ts: p.ts,
                    value: p.value,
                    seriesName: "出方向",
                    segmentName: "out-\(idx)"
                )
            }
        }
    }

    var body: some View {
        HStack(spacing: 0) {
            VStack(spacing: 0) {
                Text(yTickLabel(yAxisMax)).font(.caption2).foregroundStyle(.secondary)
                Spacer(); Text(yTickLabel(yStep * 3)).font(.caption2).foregroundStyle(.secondary)
                Spacer(); Text(yTickLabel(yStep * 2)).font(.caption2).foregroundStyle(.secondary)
                Spacer(); Text(yTickLabel(yStep)).font(.caption2).foregroundStyle(.secondary)
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
                    ForEach(yTicks, id: \.self) { tick in
                        RuleMark(y: .value("YTick", tick))
                            .foregroundStyle(Color.gray.opacity(0.22))
                            .lineStyle(StrokeStyle(lineWidth: 0.8))
                    }

                    ForEach(exportInPoints) { p in
                        LineMark(
                            x: .value("时间", p.ts),
                            y: .value("值", p.value),
                            series: .value("线段", p.segmentName)
                        )
                        .foregroundStyle(by: .value("序列", p.seriesName))
                        .lineStyle(StrokeStyle(lineWidth: 2.6))
                    }
                    ForEach(exportOutPoints) { p in
                        LineMark(
                            x: .value("时间", p.ts),
                            y: .value("值", p.value),
                            series: .value("线段", p.segmentName)
                        )
                        .foregroundStyle(by: .value("序列", p.seriesName))
                        .lineStyle(StrokeStyle(lineWidth: 2.6))
                    }
                }
                .chartForegroundStyleScale([
                    "入方向": Color(red: 0.22, green: 0.36, blue: 0.95),
                    "出方向": Color(red: 0.04, green: 0.73, blue: 0.43)
                ])
                .chartYScale(domain: 0...yAxisMax)
                .chartYAxis(.hidden)
                .chartXAxis {
                    AxisMarks(values: xAxisValues) { value in
                        AxisGridLine(); AxisTick()
                        AxisValueLabel {
                            if let date = value.as(Date.self) {
                                Text(xAxisLabel(date))
                            }
                        }
                    }
                }
                .frame(height: 520)
            }
        }
    }

    private var yTicks: [Double] {
        [0, 1, 2, 3, 4].map { Double($0) * yStep }
    }

    private var yStep: Double {
        if model.yMax >= 1_000_000 {
            let maxMbps = model.yMax / 1_000_000
            let stepMbps = max(10.0, ceil(maxMbps / 4.0 / 10.0) * 10.0)
            return stepMbps * 1_000_000
        }
        let maxKbps = model.yMax / 1_000
        let stepKbps = max(10.0, ceil(maxKbps / 4.0 / 10.0) * 10.0)
        return stepKbps * 1_000
    }

    private var yAxisMax: Double {
        yStep * 4.0
    }

    private var inboundSegments: [[TrafficChartPoint]] {
        model.inSegments.map { seg in
            seg.points.compactMap { p in
                guard let v = finite(p.traffic_in_bps) else { return nil }
                return TrafficChartPoint(ts: p.timestamp, series: .inbound, value: v)
            }
        }
    }

    private var outboundSegments: [[TrafficChartPoint]] {
        model.outSegments.map { seg in
            seg.points.compactMap { p in
                guard let v = finite(p.traffic_out_bps) else { return nil }
                return TrafficChartPoint(ts: p.timestamp, series: .outbound, value: v)
            }
        }
    }

    private func xAxisLabel(_ date: Date) -> String {
        let cal = Calendar.current
        let hm = cal.dateComponents([.hour, .minute], from: date)
        let h = hm.hour ?? 0
        let m = hm.minute ?? 0
        if h == 0 && m == 0 { return date.formatted(.dateTime.month().day()) }
        return date.formatted(.dateTime.hour().minute())
    }

    private func yTickLabel(_ value: Double) -> String {
        if yStep >= 1_000_000 {
            return "\(Int((value / 1_000_000).rounded())) Mbps"
        }
        return "\(Int((value / 1_000).rounded())) Kbps"
    }

    private var xAxisValues: [Date] {
        let times = model.decimated.map(\.timestamp)
        guard let start = times.min(), let end = times.max(), start < end else { return [] }
        let span = Int(end.timeIntervalSince(start))
        let minuteStep: Int
        switch span {
        case ..<21600: minuteStep = 30
        case ..<86400: minuteStep = 60
        case ..<259200: minuteStep = 180
        case ..<1209600: minuteStep = 360
        default: minuteStep = 720
        }
        return buildTimeAxisValues(start: start, end: end, minuteStep: minuteStep)
    }
}

private func buildTimeAxisValues(start: Date, end: Date, minuteStep: Int) -> [Date] {
    guard start < end else { return [] }
    let cal = Calendar.current
    let step = max(1, minuteStep)
    var values: [Date] = []

    var c = cal.dateComponents([.year, .month, .day, .hour, .minute], from: start)
    c.minute = ((c.minute ?? 0) / step) * step
    c.second = 0
    var cursor = cal.date(from: c) ?? start
    if cursor < start {
        cursor = cal.date(byAdding: .minute, value: step, to: cursor) ?? start
    }
    while cursor <= end {
        values.append(cursor)
        cursor = cal.date(byAdding: .minute, value: step, to: cursor) ?? end.addingTimeInterval(1)
    }

    let firstDay = cal.startOfDay(for: start)
    var midnight = cal.date(byAdding: .day, value: 1, to: firstDay) ?? firstDay
    if midnight <= start {
        midnight = cal.date(byAdding: .day, value: 1, to: midnight) ?? midnight
    }
    var midnights: [Date] = []
    while midnight <= end {
        midnights.append(midnight)
        midnight = cal.date(byAdding: .day, value: 1, to: midnight) ?? end.addingTimeInterval(1)
    }

    values.append(contentsOf: midnights)
    let merged = Array(Set(values.map { $0.timeIntervalSince1970 })).sorted()
    return merged.map { Date(timeIntervalSince1970: $0) }
}
