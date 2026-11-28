// Package social holds the social identity features of a user account:
// referral codes and first-touch attribution, and the read model of the
// organizations the user is a customer of. It does not import other modules;
// cross-module data (workspaces, CRM contacts) comes through small interfaces
// in social/service implemented by adapters in internal/app.
package social
