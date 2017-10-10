package server

import (
	log "github.com/Sirupsen/logrus"
	"github.com/disorganizer/brig/brigd/capnp"
	"github.com/disorganizer/brig/catfs"
	capnplib "zombiezen.com/go/capnproto2"
	"zombiezen.com/go/capnproto2/server"
)

type vcsHandler struct {
	base *base
}

func (vcs *vcsHandler) Log(call capnp.VCS_log) error {
	server.Ack(call.Options)
	seg := call.Results.Segment()

	return vcs.base.withOwnFs(func(fs *catfs.FS) error {
		entries, err := fs.Log()
		if err != nil {
			return err
		}

		lst, err := capnp.NewLogEntry_List(seg, int32(len(entries)))
		if err != nil {
			return err
		}

		for idx, entry := range entries {
			capEntry, err := capnp.NewLogEntry(seg)
			if err != nil {
				return err
			}

			if err := capEntry.SetHash(entry.Hash); err != nil {
				return err
			}

			modTime, err := entry.Date.MarshalText()
			if err != nil {
				return err
			}

			log.Errorf("ENTRY %v %s", entry, modTime)

			if err := capEntry.SetDate(string(modTime)); err != nil {
				return err
			}

			tagList, err := capnplib.NewTextList(seg, int32(len(entry.Tags)))
			if err != nil {
				return err
			}

			for idx, tag := range entry.Tags {
				if err := tagList.Set(idx, tag); err != nil {
					return err
				}
			}

			if err := capEntry.SetTags(tagList); err != nil {
				return err
			}

			if err := capEntry.SetMsg(entry.Msg); err != nil {
				return err
			}

			lst.Set(idx, capEntry)
		}

		return call.Results.SetEntries(lst)
	})
}

func (vcs *vcsHandler) Commit(call capnp.VCS_commit) error {
	server.Ack(call.Options)

	msg, err := call.Params.Msg()
	if err != nil {
		return err
	}

	return vcs.base.withOwnFs(func(fs *catfs.FS) error {
		return fs.MakeCommit(msg)
	})
}

func (vcs *vcsHandler) Tag(call capnp.VCS_tag) error {
	server.Ack(call.Options)

	rev, err := call.Params.Rev()
	if err != nil {
		return err
	}

	tagName, err := call.Params.TagName()
	if err != nil {
		return err
	}

	return vcs.base.withOwnFs(func(fs *catfs.FS) error {
		return fs.Tag(rev, tagName)
	})
}

func (vcs *vcsHandler) Untag(call capnp.VCS_untag) error {
	server.Ack(call.Options)

	tagName, err := call.Params.TagName()
	if err != nil {
		return err
	}

	return vcs.base.withOwnFs(func(fs *catfs.FS) error {
		return fs.RemoveTag(tagName)
	})
}

func (vcs *vcsHandler) Reset(call capnp.VCS_reset) error {
	server.Ack(call.Options)

	path, err := call.Params.Path()
	if err != nil {
		return err
	}

	rev, err := call.Params.Rev()
	if err != nil {
		return err
	}

	return vcs.base.withOwnFs(func(fs *catfs.FS) error {
		return fs.Reset(path, rev)
	})
}

func (vcs *vcsHandler) Checkout(call capnp.VCS_checkout) error {
	server.Ack(call.Options)

	rev, err := call.Params.Rev()
	if err != nil {
		return err
	}

	return vcs.base.withOwnFs(func(fs *catfs.FS) error {
		return fs.Checkout(rev, call.Params.Force())
	})
}

func (vcs *vcsHandler) History(call capnp.VCS_history) error {
	server.Ack(call.Options)

	path, err := call.Params.Path()
	if err != nil {
		return err
	}

	seg := call.Results.Segment()

	return vcs.base.withOwnFs(func(fs *catfs.FS) error {
		history, err := fs.History(path)
		if err != nil {
			return err
		}

		lst, err := capnp.NewHistoryEntry_List(seg, int32(len(history)))
		if err != nil {
			return err
		}

		for idx := 0; idx < len(history); idx++ {
			entry, err := capnp.NewHistoryEntry(seg)
			if err != nil {
				return err
			}

			if err := entry.SetPath(history[idx].Path); err != nil {
				return err
			}

			if err := entry.SetChange(history[idx].Change); err != nil {
				return err
			}

			if err := entry.SetRef(history[idx].Ref); err != nil {
				return err
			}

			if err := lst.Set(idx, entry); err != nil {
				return err
			}
		}

		return call.Results.SetHistory(lst)
	})
}
