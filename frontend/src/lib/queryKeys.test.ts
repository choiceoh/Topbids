import { describe, it, expect } from 'vitest'
import { queryKeys } from './queryKeys'

describe('queryKeys', () => {
  describe('auth', () => {
    it('me includes auth prefix', () => {
      expect(queryKeys.auth.me()).toEqual(['auth', 'me'])
    })
    it('users includes auth prefix', () => {
      expect(queryKeys.auth.users()).toEqual(['auth', 'users'])
    })
  })

  describe('collections', () => {
    it('list', () => {
      expect(queryKeys.collections.list()).toEqual(['collections', 'list'])
    })
    it('detail includes id', () => {
      expect(queryKeys.collections.detail('abc')).toEqual(['collections', 'detail', 'abc'])
    })
    it('bySlug includes slug', () => {
      expect(queryKeys.collections.bySlug('tasks')).toEqual(['collections', 'bySlug', 'tasks'])
    })
  })

  describe('entries', () => {
    it('list includes slug and query', () => {
      const key = queryKeys.entries.list('tasks', { page: 1 })
      expect(key).toEqual(['entries', 'tasks', 'list', { page: 1 }])
    })
    it('list defaults query to empty object', () => {
      const key = queryKeys.entries.list('tasks')
      expect(key).toEqual(['entries', 'tasks', 'list', {}])
    })
    it('detail includes slug and id', () => {
      expect(queryKeys.entries.detail('tasks', 'r1')).toEqual(['entries', 'tasks', 'detail', 'r1'])
    })
  })

  describe('process', () => {
    it('detail includes collection id', () => {
      expect(queryKeys.process.detail('col-1')).toEqual(['process', 'col-1'])
    })
  })

  describe('comments', () => {
    it('list includes slug and record id', () => {
      expect(queryKeys.comments.list('tasks', 'r1')).toEqual(['comments', 'tasks', 'r1'])
    })
  })

  describe('notifications', () => {
    it('list', () => {
      expect(queryKeys.notifications.list()).toEqual(['notifications', 'list'])
    })
    it('unreadCount', () => {
      expect(queryKeys.notifications.unreadCount()).toEqual(['notifications', 'unread'])
    })
  })

  describe('members', () => {
    it('list includes collection id', () => {
      expect(queryKeys.members.list('col-1')).toEqual(['members', 'list', 'col-1'])
    })
  })

  describe('history', () => {
    it('record includes slug and record id', () => {
      expect(queryKeys.history.record('tasks', 'r1')).toEqual(['history', 'tasks', 'r1'])
    })
  })

  describe('views', () => {
    it('list includes collection id', () => {
      expect(queryKeys.views.list('col-1')).toEqual(['views', 'col-1', 'list'])
    })
  })

  describe('savedViews', () => {
    it('list includes collection id', () => {
      expect(queryKeys.savedViews.list('col-1')).toEqual(['savedViews', 'col-1', 'list'])
    })
  })

  describe('migrations', () => {
    it('history with collection id', () => {
      expect(queryKeys.migrations.history('col-1')).toEqual(['migrations', 'history', 'col-1'])
    })
    it('history without collection id defaults to all', () => {
      expect(queryKeys.migrations.history()).toEqual(['migrations', 'history', 'all'])
    })
  })

  describe('automations', () => {
    it('list includes collection id', () => {
      expect(queryKeys.automations.list('col-1')).toEqual(['automations', 'list', 'col-1'])
    })
    it('detail includes id', () => {
      expect(queryKeys.automations.detail('a1')).toEqual(['automations', 'detail', 'a1'])
    })
    it('runs includes id', () => {
      expect(queryKeys.automations.runs('a1')).toEqual(['automations', 'runs', 'a1'])
    })
  })

  describe('hierarchy', () => {
    it('all keys start with their domain prefix', () => {
      expect(queryKeys.auth.all).toEqual(['auth'])
      expect(queryKeys.collections.all).toEqual(['collections'])
      expect(queryKeys.entries.all).toEqual(['entries'])
      expect(queryKeys.departments.all).toEqual(['departments'])
    })
  })
})
