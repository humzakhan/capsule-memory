package records

// OwnerProof authorizes one exact independent insight revision.
type OwnerProof struct{
 Payload string `json:"payload" cm:"required"`
 Signature string `json:"signature" cm:"required"`
}
type OwnerAssertionBody struct{
 Version int `json:"version" cm:"required"`
 Vault string `json:"vault_id" cm:"required"`
 Scope string `json:"scope_id" cm:"required"`
 Record string `json:"record_id" cm:"required"`
 Revision string `json:"revision_id" cm:"required"`
 OperationID string `json:"operation_id" cm:"required"`
 Operation string `json:"operation" cm:"required"`
 Parents []string `json:"parents" cm:"required"`
 Restores []string `json:"restores" cm:"required"`
 Author string `json:"author_device_id" cm:"required"`
 ContentSHA256 string `json:"content_sha256" cm:"required"`
}
func(v OwnerAssertionBody) Validate()error{
 if v.Version!=1||!ids.MatchString(v.Vault)||!ids.MatchString(v.Scope)||!ids.MatchString(v.Record)||!ids.MatchString(v.Revision)||!ids.MatchString(v.OperationID)||!ids.MatchString(v.Author)||!one(v.Operation,"put","resolve","restore")||len(v.Parents)>128||len(v.Restores)>128||!hashes.MatchString(v.ContentSHA256){return ErrInvalid}
 for _,id:=range append(append([]string{},v.Parents...),v.Restores...){if !ids.MatchString(id){return ErrInvalid}};return nil
}
func(v OwnerProof) Validate()error{var body OwnerAssertionBody;if signed(v.Payload,v.Signature,&body)!=nil{return ErrInvalid};return body.Validate()}
