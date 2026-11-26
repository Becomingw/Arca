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
  Workflow,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Progress,
  ProgressLabel,
  ProgressValue,
} from '@/components/ui/progress'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'

const protocols = [
  { name: 'OpenAI Responses', route: '/v1/responses', client: 'Codex' },
  { name: 'Claude Messages', route: '/v1/messages', client: 'Claude Code' },
  { name: 'Gemini API', route: '/v1beta/models', client: 'Gemini CLI' },
] as const

export function AccessSection() {
  const { t } = useTranslation()
  return (
    <section
      id='access'
      className='arca-home-section'
      aria-labelledby='access-title'
    >
      <div className='arca-section-heading'>
        <h2 id='access-title'>{t('One gateway for multiple protocols')}</h2>
        <p>
          {t(
            'Connect your existing tools through model aliases, channel routing, and protocol conversion.'
          )}
        </p>
      </div>
      <div className='arca-protocols'>
        {protocols.map((protocol) => (
          <Card key={protocol.route}>
            <CardHeader>
              <CardTitle>{protocol.client}</CardTitle>
            </CardHeader>
            <CardContent className='flex flex-col gap-2'>
              <span className='text-muted-foreground'>{protocol.name}</span>
              <code>{protocol.route}</code>
            </CardContent>
          </Card>
        ))}
      </div>
      <Button variant='outline' render={<Link to='/pricing' />}>
        {t('View Pricing')}{' '}
        <ArrowRight data-icon='inline-end' aria-hidden='true' />
      </Button>
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
      className='arca-home-section'
      aria-labelledby='entitlements-title'
    >
      <div className='arca-section-heading'>
        <h2 id='entitlements-title'>
          {t('Model access and budgets for every team')}
        </h2>
        <p>
          {t(
            'Define model families, assign access to groups, and track total and per-family budgets.'
          )}
        </p>
      </div>
      <Card className='arca-illustration'>
        <CardHeader>
          <CardTitle>{t('Example usage and budgets')}</CardTitle>
        </CardHeader>
        <CardContent>
          <Tabs defaultValue='team'>
            <TabsList aria-label={t('Member examples')} className='max-w-full'>
              {sampleMembers.map((member) => (
                <TabsTrigger key={member.id} value={member.id}>
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
      className='arca-home-section'
      aria-labelledby='operations-title'
    >
      <div className='arca-section-heading'>
        <h2 id='operations-title'>
          {t('Feedback and agent-assisted operations')}
        </h2>
        <p>
          {t(
            'Track an issue from the first report through the confirmed resolution.'
          )}
        </p>
      </div>
      <ol className='arca-operations arca-illustration'>
        {steps.map((step) => (
          <li key={step.label}>
            <step.icon aria-hidden='true' />
            <div>
              <h3>{step.label}</h3>
              <p>{step.description}</p>
            </div>
          </li>
        ))}
      </ol>
      <Button
        variant='outline'
        render={
          props.isAuthenticated ? (
            <Link to='/feedback' />
          ) : (
            <Link to='/sign-in' search={{ redirect: '/feedback' }} />
          )
        }
      >
        {t('Open feedback')}{' '}
        <ArrowRight data-icon='inline-end' aria-hidden='true' />
      </Button>
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
      className='arca-home-section'
      aria-labelledby='project-links-title'
    >
      <div className='arca-section-heading'>
        <h2 id='project-links-title'>{t('Project links')}</h2>
      </div>
      <ul className='arca-project-links'>
        {resources.map((resource) => (
          <li key={resource.href}>
            <a href={resource.href} target='_blank' rel='noopener noreferrer'>
              {resource.name} <ArrowUpRight aria-hidden='true' />
            </a>
          </li>
        ))}
      </ul>
    </section>
  )
}
