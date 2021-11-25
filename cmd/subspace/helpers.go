package main

import "sort"

func FindFirstFreeID(profiles []*Profile) uint32 {
	profileIDs := getProfileIDs(profiles)
	sort.Slice(profileIDs, func(i, j int) bool {
		return profileIDs[i] < profileIDs[j]
	})

	const minID = 2
	if len(profileIDs) == 0 {
		return minID
	}
	maxID := profileIDs[len(profileIDs)-1]
	freeID := uint32(maxID + 1)
	for i := minID; i < maxID; i++ {
		if i != profileIDs[i-minID] {
			freeID = uint32(i)
			break
		}
	}

	return freeID
}

func getProfileIDs(profiles []*Profile) []int {
	var profileIDs = make([]int, len(profiles))
	for i, profile := range profiles {
		profileIDs[i] = profile.Number
	}

	return profileIDs
}
