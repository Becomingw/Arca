import type { MessageKey } from './copy'

export const institutionLink = {
  name: 'AI4RadLab',
  href: 'https://ai4rad-ssph.github.io/',
  description: 'site.home.links.institutionDescription',
} as const

export const friendLinkGroups = [
  {
    id: 'research',
    title: 'site.home.links.research',
    links: [
      { name: 'AbdomenNet', href: 'https://github.com/FrankZhangRp/Acute_abdomen', description: 'site.home.links.abdomen' },
      { name: 'BoneCoT', href: 'https://frankzhangrp.github.io/BoneCoT/', description: 'site.home.links.bone' },
      { name: 'CGP-Plaque', href: 'https://github.com/Becomingw/CGP-Plaque', description: 'site.home.links.plaque' },
      { name: 'RescueCTA', href: 'https://github.com/Becomingw/RescueCTA', description: 'site.home.links.rescue' },
    ],
  },
  {
    id: 'opensource',
    title: 'site.home.links.opensource',
    links: [
      { name: 'Margin. 页边', href: 'https://github.com/Becomingw/Margin', description: 'site.home.links.margin' },
      { name: 'rmount-ctx', href: 'https://github.com/Becomingw/rmount-ctx', description: 'site.home.links.rmount' },
      { name: 'Aether', href: 'https://github.com/fawney19/Aether', description: 'site.home.links.aether', acknowledgement: true },
    ],
  },
] as const satisfies ReadonlyArray<{
  id: string
  title: MessageKey
  links: ReadonlyArray<{ name: string, href: string, description: MessageKey, acknowledgement?: boolean }>
}>
