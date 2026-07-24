/** 客户端品牌图标的黑白线稿版（六边形网络），随 currentColor 适配主题 */
export default function NexusLogoIcon({ size = 22 }: { size?: number }) {
  return (
    <svg viewBox="0 0 1024 1024" width={size} height={size} aria-hidden>
      <g stroke="currentColor" strokeWidth="28" strokeLinecap="round" fill="none">
        <polygon points="512,262 728.6,387 728.6,637 512,762 295.4,637 295.4,387" strokeWidth="24" />
        <line x1="512" y1="512" x2="512" y2="262" />
        <line x1="512" y1="512" x2="728.6" y2="387" />
        <line x1="512" y1="512" x2="728.6" y2="637" />
        <line x1="512" y1="512" x2="512" y2="762" />
        <line x1="512" y1="512" x2="295.4" y2="637" />
        <line x1="512" y1="512" x2="295.4" y2="387" />
      </g>
      <g fill="currentColor">
        <circle cx="512" cy="262" r="52" />
        <circle cx="728.6" cy="387" r="52" />
        <circle cx="728.6" cy="637" r="52" />
        <circle cx="512" cy="762" r="52" />
        <circle cx="295.4" cy="637" r="52" />
        <circle cx="295.4" cy="387" r="52" />
        <circle cx="512" cy="512" r="64" />
      </g>
    </svg>
  )
}
