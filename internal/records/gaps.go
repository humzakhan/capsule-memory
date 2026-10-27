package records

import "sort"

// Preserve every reason and total count within the wire bound. Exact locations
// stay in the local audit; aggregated archive coverage never claims completeness.
func MergeGaps(input []Gap)[]Gap{
 counts:=map[string]int{};for _,g:=range input{counts[g.Reason]+=g.Count}
 reasons:=make([]string,0,len(counts));for reason:=range counts{reasons=append(reasons,reason)};sort.Strings(reasons)
 out:=[]Gap{};for _,reason:=range reasons{out=append(out,Gap{Reason:reason,Count:counts[reason]})};return out
}
