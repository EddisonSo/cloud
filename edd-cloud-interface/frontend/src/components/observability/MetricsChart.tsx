import {
  AreaChart,
  Area,
  LineChart,
  Line,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
} from "recharts";

/* ─── Instrument · Ice chart tokens ────────────────────────────────────────
 * Main series:   ice  #b7d9f2  strokeWidth 1.6
 * Secondary:     dim  #8b8f94  strokeWidth 1.2
 * Tertiary:      faint #5b5f64 strokeWidth 1.2
 * Semantics:     err  #e5544b  warn #e8b43a
 * Grid/border:   #26292d   (hairline, horizontal only)
 * ───────────────────────────────────────────────────────────────────────── */
const ICE   = "#b7d9f2";
const DIM   = "#8b8f94";
const FAINT = "#5b5f64";

const BORDER = "#26292d";

const TOOLTIP_STYLE: React.CSSProperties = {
  background: "#1a1d20",
  border: "1px solid #26292d",
  borderRadius: 0,
  fontFamily: "IBM Plex Mono, JetBrains Mono, monospace",
  fontSize: 12,
};

const LABEL_STYLE: React.CSSProperties = {
  color: "#5b5f64",
  fontSize: 10,
  fontFamily: "IBM Plex Mono, monospace",
};

const ITEM_STYLE: React.CSSProperties = {
  color: "#8b8f94",
  fontSize: 11,
};

const AXIS_TICK = {
  fontSize: 10,
  fontFamily: "IBM Plex Mono, JetBrains Mono, monospace",
  fill: FAINT,
};

function formatTimestamp(timestamp: string | number): string {
  const date =
    typeof timestamp === "number"
      ? new Date(timestamp * 1000)
      : new Date(timestamp);
  if (isNaN(date.getTime())) return "";
  return date.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
}

function formatPercent(value: number): string {
  return `${value.toFixed(1)}%`;
}

/* ── Single-series area chart ──────────────────────────────────────────── */

interface MetricsChartProps {
  data: unknown[];
  title: string;
  dataKey: string;
  /** Kept for API compatibility; design language always uses ICE. */
  color: string;
  yAxisLabel?: string;
}

export function MetricsChart({
  data,
  title,
  dataKey,
  yAxisLabel = "%",
}: MetricsChartProps) {
  const gradientId = `ice-area-${dataKey}`;

  if (!data || data.length === 0) {
    return (
      <div className="h-48 flex items-center justify-center font-mono text-xs text-muted-foreground">
        No data available
      </div>
    );
  }

  return (
    <div>
      {/* Chart head */}
      <div className="flex items-center justify-between mb-3">
        <span className="microlabel">{title}</span>
      </div>
      <div className="h-44">
        <ResponsiveContainer width="100%" height="100%">
          <AreaChart
            data={data}
            margin={{ top: 4, right: 8, left: 0, bottom: 4 }}
          >
            <defs>
              <linearGradient id={gradientId} x1="0" y1="0" x2="0" y2="1">
                <stop offset="0%" stopColor={ICE} stopOpacity={0.14} />
                <stop offset="100%" stopColor={ICE} stopOpacity={0} />
              </linearGradient>
            </defs>
            <CartesianGrid
              strokeDasharray="2 4"
              stroke={BORDER}
              horizontal={true}
              vertical={false}
            />
            <XAxis
              dataKey="t"
              tickFormatter={formatTimestamp}
              tick={AXIS_TICK}
              axisLine={{ stroke: BORDER }}
              tickLine={{ stroke: BORDER }}
              interval="preserveStartEnd"
              minTickGap={60}
            />
            <YAxis
              domain={[0, 100]}
              tickFormatter={(v: number) => `${v}${yAxisLabel}`}
              tick={AXIS_TICK}
              axisLine={false}
              tickLine={false}
              width={36}
            />
            <Tooltip
              contentStyle={TOOLTIP_STYLE}
              labelStyle={LABEL_STYLE}
              itemStyle={ITEM_STYLE}
              labelFormatter={(label: unknown) =>
                formatTimestamp(label as string | number)
              }
              formatter={(value: unknown) => [
                formatPercent(value as number),
                dataKey,
              ]}
            />
            <Area
              type="monotone"
              dataKey={dataKey}
              stroke={ICE}
              strokeWidth={1.6}
              fill={`url(#${gradientId})`}
              dot={false}
              activeDot={{ r: 3, fill: ICE, strokeWidth: 0 }}
            />
          </AreaChart>
        </ResponsiveContainer>
      </div>
    </div>
  );
}

/* ── Multi-series line chart ────────────────────────────────────────────── */

interface MultiMetricsChartProps {
  data: unknown[];
  title: string;
  height?: number;
}

/** Inline legend rendered in the chart-card head — replaces recharts Legend. */
function ChartLegend() {
  const series = [
    { key: "CPU",  color: ICE },
    { key: "MEM",  color: DIM },
    { key: "DISK", color: FAINT },
  ];
  return (
    <div className="flex items-center gap-4">
      {series.map(({ key, color }) => (
        <span
          key={key}
          className="inline-flex items-center gap-1.5 font-mono text-[10px] uppercase tracking-[0.12em] text-faint"
        >
          {/* hairline swatch */}
          <span
            aria-hidden="true"
            style={{
              display: "inline-block",
              width: 16,
              height: 1.5,
              background: color,
              verticalAlign: "middle",
            }}
          />
          {key}
        </span>
      ))}
    </div>
  );
}

export function MultiMetricsChart({ data, title }: MultiMetricsChartProps) {
  if (!data || data.length === 0) {
    return (
      <div className="w-full aspect-[5/2] max-h-[600px] flex items-center justify-center font-mono text-xs text-muted-foreground">
        No data available
      </div>
    );
  }

  return (
    <div className="w-full">
      {/* Chart head: microlabel title left, inline legend right */}
      <div className="flex items-center justify-between mb-3">
        {title ? (
          <span className="microlabel">{title}</span>
        ) : (
          <span />
        )}
        <ChartLegend />
      </div>
      <div className="aspect-[5/2] max-h-[600px]">
        <ResponsiveContainer width="100%" height="100%">
          <LineChart
            data={data}
            margin={{ top: 4, right: 8, left: 0, bottom: 4 }}
          >
            <CartesianGrid
              strokeDasharray="2 4"
              stroke={BORDER}
              horizontal={true}
              vertical={false}
            />
            <XAxis
              dataKey="t"
              tickFormatter={formatTimestamp}
              tick={AXIS_TICK}
              axisLine={{ stroke: BORDER }}
              tickLine={{ stroke: BORDER }}
              interval="preserveStartEnd"
              minTickGap={80}
            />
            <YAxis
              domain={[0, 100]}
              tickFormatter={(v: number) => `${v}%`}
              tick={AXIS_TICK}
              axisLine={false}
              tickLine={false}
              width={36}
            />
            <Tooltip
              contentStyle={TOOLTIP_STYLE}
              labelStyle={LABEL_STYLE}
              itemStyle={ITEM_STYLE}
              labelFormatter={(label: unknown) =>
                formatTimestamp(label as string | number)
              }
              formatter={(value: unknown, name: unknown) => [
                formatPercent(value as number),
                String(name),
              ]}
            />
            {/* Primary: ice */}
            <Line
              type="monotone"
              dataKey="cpu"
              name="CPU"
              stroke={ICE}
              strokeWidth={1.6}
              dot={false}
              activeDot={{ r: 3, fill: ICE, strokeWidth: 0 }}
            />
            {/* Secondary: dim */}
            <Line
              type="monotone"
              dataKey="mem"
              name="MEM"
              stroke={DIM}
              strokeWidth={1.2}
              dot={false}
              activeDot={{ r: 3, fill: DIM, strokeWidth: 0 }}
            />
            {/* Tertiary: faint */}
            <Line
              type="monotone"
              dataKey="disk"
              name="DISK"
              stroke={FAINT}
              strokeWidth={1.2}
              dot={false}
              activeDot={{ r: 3, fill: FAINT, strokeWidth: 0 }}
            />
          </LineChart>
        </ResponsiveContainer>
      </div>
    </div>
  );
}
