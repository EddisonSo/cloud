/*
 * TimeRangeSelector — bare views switcher
 * Active range: ice "›" prefix + primary text.
 * Inactive: faint, hover → muted-foreground.
 * No pill chrome, no rounded containers, no background fills.
 */

interface TimeRange {
  value: string;
  label: string;
}

const timeRanges: TimeRange[] = [
  { value: "1h",  label: "1H" },
  { value: "6h",  label: "6H" },
  { value: "24h", label: "24H" },
  { value: "7d",  label: "7D" },
];

interface TimeRangeSelectorProps {
  value: string;
  onChange: (value: string) => void;
}

export function TimeRangeSelector({
  value,
  onChange,
}: TimeRangeSelectorProps): React.ReactElement {
  return (
    <div className="inline-flex items-center gap-5">
      {timeRanges.map((range) => {
        const active = range.value === value;
        return (
          <button
            key={range.value}
            onClick={() => onChange(range.value)}
            className={`font-mono text-[10.5px] uppercase tracking-[0.14em] transition-colors ${
              active
                ? "text-primary"
                : "text-faint hover:text-muted-foreground"
            }`}
          >
            {active && <span className="mr-1" aria-hidden="true">›</span>}
            {range.label}
          </button>
        );
      })}
    </div>
  );
}
