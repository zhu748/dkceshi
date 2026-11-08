import { formatCooldownUntil, isAccountCoolingDown } from './accountCooldown.js'

export default function AccountCooldownBadge({ account, t }) {
    if (account?.banned || account?.muted || account?.manual_disabled || !isAccountCoolingDown(account)) return null
    return <span className="font-mono bg-amber-500/10 text-amber-500 px-1.5 py-0.5 rounded text-[10px]" title={t('accountManager.accountCooldownHint')}>
        {t('accountManager.accountCooldownRecoverAt', { time: formatCooldownUntil(account.cooldown_until) })}
    </span>
}
