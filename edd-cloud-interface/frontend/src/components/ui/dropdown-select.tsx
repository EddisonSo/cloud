import * as React from "react";
import { createPortal } from "react-dom";
import { ChevronDown, Search, Check } from "lucide-react";
import { cn } from "@/lib/utils";

export interface DropdownOption {
  value: string;
  label: string;
  icon?: React.ReactNode;
}

export interface DropdownGroup {
  label: string;
  options: DropdownOption[];
}

interface DropdownSelectProps {
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  searchable?: boolean;
  options?: DropdownOption[];
  groups?: DropdownGroup[];
  className?: string;
  disabled?: boolean;
}

export function DropdownSelect({
  value,
  onChange,
  placeholder = "Select...",
  searchable = false,
  options,
  groups,
  className,
  disabled = false,
}: DropdownSelectProps) {
  const [open, setOpen] = React.useState(false);
  const [search, setSearch] = React.useState("");
  const [highlightIndex, setHighlightIndex] = React.useState(-1);
  const [dropdownStyle, setDropdownStyle] = React.useState<React.CSSProperties>({});

  const triggerRef = React.useRef<HTMLButtonElement>(null);
  const searchRef = React.useRef<HTMLInputElement>(null);
  const listRef = React.useRef<HTMLDivElement>(null);

  // Build flat list of all options for keyboard nav and display
  const allOptions: DropdownOption[] = React.useMemo(() => {
    if (groups) {
      return groups.flatMap((g) => g.options);
    }
    return options ?? [];
  }, [options, groups]);

  // Filter by search
  const filteredOptions: DropdownOption[] = React.useMemo(() => {
    if (!search.trim()) return allOptions;
    const lower = search.toLowerCase();
    return allOptions.filter((o) => o.label.toLowerCase().includes(lower));
  }, [allOptions, search]);

  // Build filtered groups for rendering
  const filteredGroups: DropdownGroup[] | null = React.useMemo(() => {
    if (!groups) return null;
    if (!search.trim()) return groups;
    const lower = search.toLowerCase();
    return groups
      .map((g) => ({
        ...g,
        options: g.options.filter((o) => o.label.toLowerCase().includes(lower)),
      }))
      .filter((g) => g.options.length > 0);
  }, [groups, search]);

  const selectedLabel = React.useMemo(() => {
    const found = allOptions.find((o) => o.value === value);
    return found ? found.label : null;
  }, [allOptions, value]);

  const openDropdown = () => {
    if (disabled) return;
    if (triggerRef.current) {
      const rect = triggerRef.current.getBoundingClientRect();
      const spaceBelow = window.innerHeight - rect.bottom;
      const dropdownHeight = Math.min(300, filteredOptions.length * 36 + (searchable ? 44 : 8) + 8);
      const showAbove = spaceBelow < dropdownHeight && rect.top > dropdownHeight;

      setDropdownStyle({
        position: "fixed",
        left: rect.left,
        width: rect.width,
        zIndex: 9999,
        ...(showAbove
          ? { bottom: window.innerHeight - rect.top }
          : { top: rect.bottom + 4 }),
      });
    }
    setOpen(true);
    setHighlightIndex(-1);
    setSearch("");
  };

  const closeDropdown = () => {
    setOpen(false);
    setSearch("");
    setHighlightIndex(-1);
  };

  const selectOption = (val: string) => {
    onChange(val);
    closeDropdown();
    triggerRef.current?.focus();
  };

  // Focus search input when dropdown opens
  React.useEffect(() => {
    if (open && searchable && searchRef.current) {
      searchRef.current.focus();
    }
  }, [open, searchable]);

  // Click outside to close
  React.useEffect(() => {
    if (!open) return;
    const handleMouseDown = (e: MouseEvent) => {
      if (
        triggerRef.current &&
        !triggerRef.current.contains(e.target as Node) &&
        listRef.current &&
        !listRef.current.contains(e.target as Node)
      ) {
        closeDropdown();
      }
    };
    document.addEventListener("mousedown", handleMouseDown);
    return () => document.removeEventListener("mousedown", handleMouseDown);
  }, [open]);

  // Keyboard navigation on the trigger button
  const handleTriggerKeyDown = (e: React.KeyboardEvent<HTMLButtonElement>) => {
    if (e.key === "Enter" || e.key === " " || e.key === "ArrowDown") {
      e.preventDefault();
      openDropdown();
    }
  };

  // Keyboard navigation inside the dropdown
  const handleDropdownKeyDown = (e: React.KeyboardEvent<HTMLDivElement>) => {
    if (e.key === "Escape") {
      e.preventDefault();
      closeDropdown();
      triggerRef.current?.focus();
      return;
    }
    if (e.key === "ArrowDown") {
      e.preventDefault();
      setHighlightIndex((prev) => Math.min(prev + 1, filteredOptions.length - 1));
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      setHighlightIndex((prev) => Math.max(prev - 1, 0));
    } else if (e.key === "Enter") {
      e.preventDefault();
      if (highlightIndex >= 0 && highlightIndex < filteredOptions.length) {
        selectOption(filteredOptions[highlightIndex].value);
      }
    }
  };

  // Scroll highlighted item into view
  React.useEffect(() => {
    if (!listRef.current || highlightIndex < 0) return;
    const items = listRef.current.querySelectorAll<HTMLElement>("[data-option-index]");
    const item = items[highlightIndex];
    if (item) {
      item.scrollIntoView({ block: "nearest" });
    }
  }, [highlightIndex]);

  const dropdownContent = open
    ? createPortal(
        <div
          ref={listRef}
          style={dropdownStyle}
          className="rounded-md border border-input bg-popover text-popover-foreground shadow-lg overflow-hidden flex flex-col"
          onKeyDown={handleDropdownKeyDown}
        >
          {searchable && (
            <div className="flex items-center gap-2 px-3 py-2 border-b border-input">
              <Search className="w-3.5 h-3.5 text-muted-foreground shrink-0" />
              <input
                ref={searchRef}
                type="text"
                value={search}
                onChange={(e) => {
                  setSearch(e.target.value);
                  setHighlightIndex(-1);
                }}
                placeholder="Search..."
                className="w-full bg-transparent text-sm outline-none placeholder:text-muted-foreground"
              />
            </div>
          )}
          <div className="overflow-y-auto max-h-[260px] py-1">
            {filteredGroups
              ? filteredGroups.map((group) => (
                  <div key={group.label}>
                    <div className="px-3 py-1.5 text-xs font-semibold text-muted-foreground uppercase tracking-wider">
                      {group.label}
                    </div>
                    {group.options.map((opt) => {
                      const flatIndex = filteredOptions.indexOf(opt);
                      return (
                        <OptionItem
                          key={opt.value}
                          option={opt}
                          isSelected={opt.value === value}
                          isHighlighted={flatIndex === highlightIndex}
                          dataIndex={flatIndex}
                          onSelect={selectOption}
                          onMouseEnter={() => setHighlightIndex(flatIndex)}
                        />
                      );
                    })}
                  </div>
                ))
              : filteredOptions.map((opt, idx) => (
                  <OptionItem
                    key={opt.value}
                    option={opt}
                    isSelected={opt.value === value}
                    isHighlighted={idx === highlightIndex}
                    dataIndex={idx}
                    onSelect={selectOption}
                    onMouseEnter={() => setHighlightIndex(idx)}
                  />
                ))}
            {filteredOptions.length === 0 && (
              <div className="px-3 py-4 text-center text-sm text-muted-foreground">
                No options found
              </div>
            )}
          </div>
        </div>,
        document.body
      )
    : null;

  return (
    <>
      <button
        ref={triggerRef}
        type="button"
        disabled={disabled}
        onClick={open ? closeDropdown : openDropdown}
        onKeyDown={handleTriggerKeyDown}
        aria-haspopup="listbox"
        aria-expanded={open}
        className={cn(
          "flex h-9 w-full items-center justify-between rounded-md border border-input bg-background px-3 py-1 text-sm shadow-sm transition-colors",
          "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
          "disabled:cursor-not-allowed disabled:opacity-50",
          !selectedLabel && "text-muted-foreground",
          className
        )}
      >
        <span className="truncate">
          {selectedLabel ?? placeholder}
        </span>
        <ChevronDown
          className={cn(
            "w-4 h-4 shrink-0 text-muted-foreground transition-transform duration-200",
            open && "rotate-180"
          )}
        />
      </button>
      {dropdownContent}
    </>
  );
}

interface OptionItemProps {
  option: DropdownOption;
  isSelected: boolean;
  isHighlighted: boolean;
  dataIndex: number;
  onSelect: (value: string) => void;
  onMouseEnter: () => void;
}

function OptionItem({
  option,
  isSelected,
  isHighlighted,
  dataIndex,
  onSelect,
  onMouseEnter,
}: OptionItemProps) {
  return (
    <div
      role="option"
      aria-selected={isSelected}
      data-option-index={dataIndex}
      onClick={() => onSelect(option.value)}
      onMouseEnter={onMouseEnter}
      className={cn(
        "flex items-center gap-2 px-3 py-2 text-sm cursor-pointer select-none",
        isHighlighted && "bg-accent text-accent-foreground",
        !isHighlighted && isSelected && "text-primary"
      )}
    >
      {option.icon && <span className="shrink-0">{option.icon}</span>}
      <span className="flex-1 truncate">{option.label}</span>
      {isSelected && <Check className="w-3.5 h-3.5 shrink-0 text-primary" />}
    </div>
  );
}
