type Tab<T extends string> = {
  key: T
  label: string
  caption?: string
}

type Props<T extends string> = {
  tabs: Tab<T>[]
  value: T
  onChange: (value: T) => void
  className?: string
}

export function PillTabs<T extends string>({ tabs, value, onChange, className }: Props<T>) {
  return (
    <div className={className ? `pill-tabs ${className}` : 'pill-tabs'} role="tablist">
      {tabs.map((tab) => {
        const active = tab.key === value
        return (
          <button
            key={tab.key}
            aria-selected={active}
            className={`pill-tab ${active ? 'pill-tab-active' : ''}`}
            onClick={() => onChange(tab.key)}
            role="tab"
            type="button"
          >
            <strong>{tab.label}</strong>
            {tab.caption ? <span>{tab.caption}</span> : null}
          </button>
        )
      })}
    </div>
  )
}
