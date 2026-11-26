/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { Link } from '@tanstack/react-router'
import {
  ArrowRight,
  ArrowUpRight,
  GitBranch,
  MessagesSquare,
  Terminal,
  Workflow,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { buttonVariants } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Progress,
  ProgressLabel,
  ProgressValue,
} from '@/components/ui/progress'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { cn } from '@/lib/utils'

const protocols = [
  { name: 'OpenAI Responses', route: '/v1/responses', client: 'Codex' },
  { name: 'Claude Messages', route: '/v1/messages', client: 'Claude Code' },
  { name: 'Gemini API', route: '/v1beta/models', client: 'Gemini CLI' },
] as const

const featureHeading = 'text-2xl leading-snug font-semibold tracking-tight'
const featureSurface = 'bg-card/65'

export function AccessSection() {
  const { t } = useTranslation()
  return (
    <section
      id='access'
      className='grid scroll-mt-24 items-center gap-5 lg:grid-cols-[minmax(0,0.85fr)_minmax(0,1.15fr)] lg:gap-12'
      aria-labelledby='access-title'
    >
      <div className='space-y-2'>
        <h2 id='access-title' className={featureHeading}>
          {t('One gateway for multiple protocols')}
        </h2>
        <p className='text-muted-foreground max-w-xl text-sm leading-relaxed'>
          {t(
            'Connect your existing tools through model aliases, channel routing, and protocol conversion.'
          )}
        </p>
      </div>
      <Card className={featureSurface}>
        <CardContent>
          <ul className='divide-border/70 divide-y'>
            {protocols.map((protocol) => (
              <li
                key={protocol.route}
                className='flex min-w-0 flex-wrap items-center gap-x-3 gap-y-2 py-3 first:pt-0 last:pb-0'
              >
                <span className='bg-muted flex size-8 shrink-0 items-center justify-center rounded-lg'>
                  <Terminal
                    className='text-muted-foreground size-4'
                    aria-hidden='true'
                  />
                </span>
                <div className='min-w-0 flex-1'>
                  <p className='text-sm font-medium'>{protocol.client}</p>
                  <p className='text-muted-foreground text-xs'>
                    {protocol.name}
                  </p>
                </div>
                <code className='bg-muted text-foreground max-w-full rounded-md px-2 py-1 text-xs break-all'>
                  {protocol.route}
                </code>
              </li>
            ))}
          </ul>
        </CardContent>
      </Card>
    </section>
  )
}

export function EntitlementsSection() {
  const { t } = useTranslation()
  const sampleMembers = [
    { id: 'team', label: t('Team member'), total: 48, openai: 56, claude: 36 },
    {
      id: 'research',
      label: t('Research member'),
      total: 31,
      openai: 18,
      claude: 67,
    },
  ]
  return (
    <section
      id='entitlements'
      className='flex min-w-0 scroll-mt-24 flex-col gap-4'
      aria-labelledby='entitlements-title'
    >
      <div className='space-y-2'>
        <h2 id='entitlements-title' className={featureHeading}>
          {t('Model access and budgets for every team')}
        </h2>
        <p className='text-muted-foreground text-sm leading-relaxed'>
          {t(
            'Define model families, assign access to groups, and track total and per-family budgets.'
          )}
        </p>
      </div>
      <Card className={featureSurface}>
        <CardHeader>
          <CardTitle>{t('Example usage and budgets')}</CardTitle>
        </CardHeader>
        <CardContent>
          <Tabs defaultValue='team'>
            <TabsList
              aria-label={t('Member examples')}
              className='h-auto max-w-full flex-wrap justify-start'
            >
              {sampleMembers.map((member) => (
                <TabsTrigger
                  key={member.id}
                  value={member.id}
                  className='min-h-7 whitespace-normal'
                >
                  {member.label}
                </TabsTrigger>
              ))}
            </TabsList>
            {sampleMembers.map((member) => (
              <TabsContent
                className='mt-3 grid gap-4'
                key={member.id}
                value={member.id}
              >
                {[
                  [t('Usage'), member.total],
                  ['OpenAI', member.openai],
                  ['Claude', member.claude],
                ].map(([label, value]) => (
                  <Progress key={label} value={Number(value)}>
                    <ProgressLabel>{label}</ProgressLabel>
                    <ProgressValue />
                  </Progress>
                ))}
              </TabsContent>
            ))}
          </Tabs>
        </CardContent>
      </Card>
    </section>
  )
}

export function OperationsSection(props: { isAuthenticated: boolean }) {
  const { t } = useTranslation()
  const steps = [
    {
      label: t('Submit feedback'),
      description: t(
        'Discuss a request and share its redacted diagnostic summary.'
      ),
      icon: MessagesSquare,
    },
    {
      label: t('Review diagnostics'),
      description: t(
        'Let an authorized agent inspect the feedback and request evidence through MCP.'
      ),
      icon: Workflow,
    },
    {
      label: t('Confirm a resolution'),
      description: t('Preview a reply or status change before confirming it.'),
      icon: GitBranch,
    },
  ]
  return (
    <section
      id='operations'
      className='flex min-w-0 scroll-mt-24 flex-col gap-4'
      aria-labelledby='operations-title'
    >
      <div className='space-y-2'>
        <h2 id='operations-title' className={featureHeading}>
          {t('Feedback and agent-assisted operations')}
        </h2>
        <p className='text-muted-foreground text-sm leading-relaxed'>
          {t(
            'Track an issue from the first report through the confirmed resolution.'
          )}
        </p>
      </div>
      <Card className={featureSurface}>
        <CardContent>
          <ol className='divide-border/70 divide-y'>
            {steps.map((step) => (
              <li
                key={step.label}
                className='flex gap-3 py-3 first:pt-0 last:pb-0'
              >
                <span className='bg-muted flex size-8 shrink-0 items-center justify-center rounded-lg'>
                  <step.icon
                    className='text-muted-foreground size-4'
                    aria-hidden='true'
                  />
                </span>
                <div>
                  <h3 className='text-sm font-medium'>{step.label}</h3>
                  <p className='text-muted-foreground mt-0.5 text-xs leading-relaxed'>
                    {step.description}
                  </p>
                </div>
              </li>
            ))}
          </ol>
          <Link
            to={props.isAuthenticated ? '/feedback' : '/sign-in'}
            search={
              props.isAuthenticated ? undefined : { redirect: '/feedback' }
            }
            className={cn(buttonVariants({ variant: 'outline' }), 'mt-4')}
          >
            {t('Open feedback')}
            <ArrowRight data-icon='inline-end' aria-hidden='true' />
          </Link>
        </CardContent>
      </Card>
    </section>
  )
}

const resources = [
  { name: 'Arca', href: 'https://github.com/Becomingw/Arca' },
  { name: 'New API', href: 'https://github.com/QuantumNous/new-api' },
  { name: 'AI4RadLab', href: 'https://ai4rad-ssph.github.io/' },
  { name: 'Aether', href: 'https://github.com/fawney19/Aether' },
  { name: 'Margin', href: 'https://github.com/Becomingw/Margin' },
  { name: 'rmount-ctx', href: 'https://github.com/Becomingw/rmount-ctx' },
]

export function LinksSection() {
  const { t } = useTranslation()
  return (
    <section
      id='project-links'
      className='border-border/70 scroll-mt-24 space-y-4 border-t pt-6 sm:pt-8'
      aria-labelledby='project-links-title'
    >
      <h2 id='project-links-title' className={featureHeading}>
        {t('Project links')}
      </h2>
      <ul className='grid grid-cols-2 gap-2.5 sm:grid-cols-3 lg:grid-cols-6'>
        {resources.map((resource) => (
          <li key={resource.href} className='min-w-0'>
            <a
              href={resource.href}
              target='_blank'
              rel='noopener noreferrer'
              className={cn(
                buttonVariants({ size: 'lg', variant: 'outline' }),
                'bg-card/65 h-auto min-h-10 w-full justify-between px-3 py-2 text-start break-words whitespace-normal'
              )}
            >
              {resource.name}{' '}
              <ArrowUpRight
                aria-hidden='true'
                className='text-muted-foreground size-3.5'
              />
            </a>
          </li>
        ))}
      </ul>
    </section>
  )
}
