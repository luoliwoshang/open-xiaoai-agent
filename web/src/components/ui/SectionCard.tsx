import type { ReactNode } from 'react'

type Props = {
  eyebrow?: string
  title?: string
  description?: string
  actions?: ReactNode
  children?: ReactNode
  className?: string
}

export function SectionCard({ eyebrow, title, description, actions, children, className }: Props) {
  return (
    <section className={`section-card ${className ?? ''}`.trim()}>
      {(eyebrow || title || description || actions) ? (
        <header className="section-card-head">
          <div className="section-card-copy">
            {eyebrow ? <p className="section-eyebrow">{eyebrow}</p> : null}
            {title ? <h3 className="section-title">{title}</h3> : null}
            {description ? <p className="section-description">{description}</p> : null}
          </div>
          {actions ? <div className="section-card-actions">{actions}</div> : null}
        </header>
      ) : null}

      {children ? <div className="section-card-body">{children}</div> : null}
    </section>
  )
}
