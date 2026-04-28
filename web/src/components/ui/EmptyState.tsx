type Props = {
  title: string
  description: string
}

export function EmptyState({ title, description }: Props) {
  return (
    <div className="empty-state">
      <strong>{title}</strong>
      <p>{description}</p>
    </div>
  )
}
